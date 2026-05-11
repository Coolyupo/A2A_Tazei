package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func analyzeWithGemini(filename, content string) (string, error) {
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

	prompt := fmt.Sprintf(`你是一個專業的安全分析 Agent。
以下是一份被 Agent 1 標記為異常的文字檔案（檔名：%s），請進行深入分析：

--- 檔案內容開始 ---
%s
--- 檔案內容結束 ---

請提供以下分析：
1. **內容摘要**：簡述檔案主要內容
2. **異常指標**：列出發現的可疑或異常特徵（若無則說明）
3. **風險評估**：整體風險等級（低 / 中 / 高）並說明理由
4. **建議處置**：具體的後續行動建議`, filename, content)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("Gemini 生成失敗：%w", err)
	}

	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("Gemini 未回傳任何候選結果")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini 回傳內容為空")
	}

	if text, ok := candidate.Content.Parts[0].(genai.Text); ok {
		return string(text), nil
	}

	return fmt.Sprintf("%v", candidate.Content.Parts[0]), nil
}
