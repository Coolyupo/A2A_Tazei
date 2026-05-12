package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func analyzeWithGemini(alertContent string) (string, error) {
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
