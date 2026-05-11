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
		Name:        "ImageAnalyzerAgent",
		Description: "接收圖片檔案，使用 Gemini Vision 進行異常分析",
		URL:         selfURL,
		Version:     "1.0.0",
		Capabilities: Capabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: []AgentSkill{
			{
				ID:          "analyze_image",
				Name:        "Image Anomaly Analysis",
				Description: "分析圖片內容，判斷是否有異常（支援 jpg/png/gif/bmp/webp）",
				InputModes:  []string{"image"},
				OutputModes: []string{"text"},
			},
		},
	}

	if err := doRegister(registryURL, card); err != nil {
		log.Printf("[Agent3] 注冊失敗：%v（將繼續啟動）", err)
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
	log.Printf("[Agent3] 已向 Registry 注冊 (%s)", registryURL)
	return nil
}

func heartbeatLoop(registryURL, selfURL string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		body, _ := json.Marshal(map[string]string{"url": selfURL})
		resp, err := http.Post(registryURL+"/agents/heartbeat", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[Agent3] 心跳失敗：%v", err)
			continue
		}
		resp.Body.Close()
	}
}
