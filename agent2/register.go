package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func startRegistration(selfURL, registryURL string) {
	card := AgentCard{
		Name:        "CriticalAlertAnalyzerAgent",
		Description: "接收 Alertmanager Critical 告警，使用 Gemini 進行深度根因分析",
		URL:         selfURL,
		Version:     "4.0.0",
		Capabilities: Capabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: []AgentSkill{
			{
				ID:          "analyze_critical",
				Name:        "Critical Alert Analysis",
				Description: "分析 Alertmanager Critical 告警，評估影響範圍並給出緊急處置建議",
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
		},
	}

	if err := doRegister(registryURL, card); err != nil {
		log.Printf("[Agent2] 注冊失敗：%v（將繼續啟動）", err)
	}

	go heartbeatLoop(registryURL, selfURL)
}

func doRegister(registryURL string, card AgentCard) error {
	body, _ := json.Marshal(card)
	resp, err := http.Post(registryURL+"/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("無法連線到 Registry：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Registry 回傳 %d", resp.StatusCode)
	}
	log.Printf("[Agent2] 已向 Registry 注冊 (%s)", registryURL)
	return nil
}

func heartbeatLoop(registryURL, selfURL string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		body, _ := json.Marshal(map[string]string{"url": selfURL})
		resp, err := http.Post(registryURL+"/agents/heartbeat", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[Agent2] 心跳失敗：%v", err)
			continue
		}
		resp.Body.Close()
	}
}
