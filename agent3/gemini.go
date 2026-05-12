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

func analyzeAndDecide(alertContent string) (*AnalysisResult, error) {
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

	// 強制 Gemini 輸出 JSON，並定義 schema 確保欄位存在
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"decision": {
				Type:        genai.TypeString,
				Description: "silence 或 escalate",
				Enum:        []string{"silence", "escalate"},
			},
			"reason": {
				Type:        genai.TypeString,
				Description: "一句話說明決策理由",
			},
			"analysis": {
				Type:        genai.TypeString,
				Description: "完整分析報告",
			},
		},
		Required: []string{"decision", "reason", "analysis"},
	}

	prompt := fmt.Sprintf(`你是一個專業的 SRE（Site Reliability Engineer），負責自主判斷 Warning 告警是否需要人工介入。
以下是一筆來自 Alertmanager 的 **Warning** 級別告警：

--- 告警內容開始 ---
%s
--- 告警內容結束 ---

請進行完整分析並做出自主決策：

決策標準：
- **silence**：告警屬於短暫性、已知問題、維護窗口內、或預計 48 小時內自動恢復的情況，不需要人工立即介入
- **escalate**：告警可能演變為 Critical、影響服務可用性、需要工程師立即確認，或不確定嚴重程度

analysis 欄位請包含：1.告警摘要 2.趨勢判斷 3.潛在風險 4.預防建議 5.觀察指標`, alertContent)

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

	// 從回應中萃取 JSON 物件（處理 Gemini 萬一仍夾帶前後文字的情況）
	rawText = extractJSON(rawText)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(rawText), &result); err != nil {
		return nil, fmt.Errorf("解析 Gemini JSON 失敗：%w（原文：%.200s）", err, rawText)
	}

	if result.Decision != "silence" && result.Decision != "escalate" {
		result.Decision = "escalate"
	}

	return &result, nil
}

// extractJSON 從任意文字中找出第一個完整的 JSON 物件
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}

	// 從 start 往後找對應的結尾 }，處理巢狀大括號
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

	// 找不到完整 JSON 物件，回傳去掉前綴的文字
	return strings.TrimSpace(s[start:])
}
