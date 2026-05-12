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

	pollInterval, err := time.ParseDuration(pollIntervalStr)
	if err != nil {
		log.Fatalf("[Agent1] 無效的 POLL_INTERVAL：%v", err)
	}

	log.Printf("[Agent1] Registry URL：%s", registryURL)
	log.Printf("[Agent1] Alertmanager URL：%s", alertmanagerURL)
	log.Printf("[Agent1] 輪詢間隔：%s", pollInterval)

	registry := NewRegistryClient(registryURL)

	if err := StartAlertmanagerPoller(alertmanagerURL, pollInterval, registry); err != nil {
		log.Fatalf("[Agent1] 監控錯誤：%v", err)
	}
}
