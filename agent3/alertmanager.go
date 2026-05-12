package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type silenceMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type silenceRequest struct {
	Matchers  []silenceMatcher `json:"matchers"`
	StartsAt  string           `json:"startsAt"`
	EndsAt    string           `json:"endsAt"`
	CreatedBy string           `json:"createdBy"`
	Comment   string           `json:"comment"`
}

type silenceResponse struct {
	SilenceID string `json:"silenceID"`
}

func silenceAlert(labels map[string]string, duration time.Duration, reason string) (string, error) {
	alertmanagerURL := getEnv("ALERTMANAGER_URL", "http://localhost:9093")

	matchers := make([]silenceMatcher, 0, len(labels))
	for k, v := range labels {
		matchers = append(matchers, silenceMatcher{
			Name:    k,
			Value:   v,
			IsRegex: false,
			IsEqual: true,
		})
	}

	now := time.Now()
	req := silenceRequest{
		Matchers:  matchers,
		StartsAt:  now.UTC().Format(time.RFC3339),
		EndsAt:    now.Add(duration).UTC().Format(time.RFC3339),
		CreatedBy: "Agent3-AI",
		Comment:   fmt.Sprintf("自動 Silence（48小時）：%s", reason),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("序列化 Silence 請求失敗：%w", err)
	}

	resp, err := http.Post(alertmanagerURL+"/api/v2/silences", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("呼叫 Alertmanager Silence API 失敗：%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("Alertmanager 回傳 HTTP %d", resp.StatusCode)
	}

	var silenceResp silenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&silenceResp); err != nil {
		return "", fmt.Errorf("解析 Silence 回應失敗：%w", err)
	}

	return silenceResp.SilenceID, nil
}
