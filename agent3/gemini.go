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
以下是一筆來自 Alertmanager 的 **Warning** 級別告警，請進行趨勢分析與預防性評估：

--- 告警內容開始 ---
%s
--- 告警內容結束 ---

請提供以下分析：
1. **告警摘要**：說明此告警的核心問題是什麼
2. **趨勢判斷**：此 Warning 告警是否可能演變為 Critical？說明判斷依據
3. **潛在風險**：若不處理，預計可能發生的後果
4. **預防建議**：在問題惡化前應採取的預防措施（優先順序排列）
5. **觀察指標**：建議持續監控哪些指標以掌握狀況變化`, alertContent)

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
