package main

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()

	alertmanagerURL := getEnv("ALERTMANAGER_URL", "http://localhost:9093")
	registryURL := getEnv("REGISTRY_URL", "http://localhost:9000")
	pollIntervalStr := getEnv("POLL_INTERVAL", "30s")
	slackWebhook := getEnv("SLACK_WEBHOOK_URL", "")
	botToken := getEnv("SLACK_BOT_TOKEN", "")
	appToken := getEnv("SLACK_APP_TOKEN", "")
	channelID := getEnv("SLACK_CHANNEL_ID", "")

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		log.Fatalf("[Agent1] 無效的 POLL_INTERVAL：%v", err)
	}

	log.Printf("[Agent1] Registry URL：%s", registryURL)
	log.Printf("[Agent1] Alertmanager URL：%s", alertmanagerURL)
	log.Printf("[Agent1] 輪詢間隔：%s", pollInterval)

	if slackWebhook != "" {
		log.Printf("[Agent1] Slack Webhook：已設定")
	} else {
		log.Printf("[Agent1] Slack Webhook：未設定")
	}

	registry := NewRegistryClient(registryURL)
	am := NewApprovalManager()

	// 啟動 Slack Socket Mode Bot（需要 BOT_TOKEN + APP_TOKEN + CHANNEL_ID）
	var bot *SlackBot
	if botToken != "" && appToken != "" {
		bot = NewSlackBot(botToken, appToken, am)
		bot.Start()
		log.Printf("[Agent1] Slack Bot：已啟動（channel: %s）", channelID)
	} else {
		log.Printf("[Agent1] Slack Bot：未設定（SLACK_BOT_TOKEN / SLACK_APP_TOKEN 未填），Critical 告警將自動核准")
	}

	if err := StartAlertmanagerPoller(alertmanagerURL, pollInterval, registry, slackWebhook, bot, am, channelID); err != nil {
		log.Fatalf("[Agent1] 監控錯誤：%v", err)
	}
}
