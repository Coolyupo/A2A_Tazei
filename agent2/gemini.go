package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// selectToolWithGemini 讓 Gemini 分析告警並從可用 MCP 工具中選擇最適合的工具
// 若無合適工具則回傳 builtin
func selectToolWithGemini(alertContent string, mcpTools []MCPTool) (*ToolChoice, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY 未設定")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("建立 Gemini 客戶端失敗：%w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash-lite")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"tool":        {Type: genai.TypeString, Description: "選定的工具名稱，若無合適工具則填 builtin"},
			"source":      {Type: genai.TypeString, Enum: []string{"mcp", "builtin"}, Description: "工具來源"},
			"args":        {Type: genai.TypeObject, Description: "工具執行參數（key-value 字串）"},
			"reason":      {Type: genai.TypeString, Description: "選擇此工具的理由"},
			"description": {Type: genai.TypeString, Description: "此工具將執行的操作說明"},
		},
		Required: []string{"tool", "source", "reason", "description"},
	}

	// 建立可用工具清單描述
	var toolsDesc strings.Builder
	if len(mcpTools) == 0 {
		toolsDesc.WriteString("（目前無可用的 MCP 工具，請使用 builtin）")
	} else {
		for i, t := range mcpTools {
			toolsDesc.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, t.Name, t.Description))
			if len(t.InputSchema) > 0 {
				toolsDesc.WriteString(fmt.Sprintf("   Schema: %s\n", string(t.InputSchema)))
			}
		}
	}

	prompt := fmt.Sprintf(`你是一個專業的 SRE，負責處理 Critical 告警。
以下是一筆緊急告警，請分析並選擇最適合的處理工具。

--- 告警內容 ---
%s
--- 告警內容結束 ---

可用 MCP 工具：
%s

內建工具（builtin）：純文字根因分析，適用於所有情況，不執行任何操作。

請選擇最適合的工具：
- 若有 MCP 工具能直接緩解或診斷此告警，優先選用
- 若無合適的 MCP 工具，選用 builtin 進行分析
- args 中只填寫工具實際需要的參數（字串格式）
- description 說明選定工具將採取的具體行動`, alertContent, toolsDesc.String())

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("Gemini 生成失敗：%w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini 未回傳內容")
	}

	rawText := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])
	if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		rawText = string(text)
	}
	rawText = extractJSON(rawText)

	var choice ToolChoice
	if err := json.Unmarshal([]byte(rawText), &choice); err != nil {
		return nil, fmt.Errorf("解析 Gemini 工具選擇失敗：%w（原文：%.200s）", err, rawText)
	}

	if choice.Source != "mcp" && choice.Source != "builtin" {
		choice.Source = "builtin"
		choice.Tool = "builtin"
	}
	if choice.Source == "builtin" {
		choice.Tool = "builtin"
	}
	if choice.Args == nil {
		choice.Args = map[string]string{}
	}

	return &choice, nil
}

// executeBuiltinAnalysis 使用 Gemini 進行純文字根因分析（內建工具）
func executeBuiltinAnalysis(alertContent string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY 未設定")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", fmt.Errorf("建立 Gemini 客戶端失敗：%w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash-lite")

	prompt := fmt.Sprintf(`你是一個專業的 SRE（Site Reliability Engineer）。
以下是一筆來自 Alertmanager 的 **Critical** 級別告警，請立即進行緊急分析：

--- 告警內容開始 ---
%s
--- 告警內容結束 ---

請提供以下分析：
1. **告警摘要**：說明此告警的核心問題是什麼
2. **影響評估**：此 Critical 告警可能影響的服務或系統範圍
3. **根因推測**：基於標籤與 Annotations 推測最可能的根因
4. **緊急處置**：立即應採取的緊急行動（優先順序排列）
5. **後續追蹤**：問題緩解後需要進行的後續排查步驟`, alertContent)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("Gemini 生成失敗：%w", err)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini 未回傳內容")
	}
	if text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		return string(text), nil
	}
	return fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0]), nil
}

// extractJSON 從任意文字中找出第一個完整的 JSON 物件（對應 agent3 的同名函式）
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[start : i+1])
			}
		}
	}
	return strings.TrimSpace(s[start:])
}
