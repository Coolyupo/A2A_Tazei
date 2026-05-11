package main

import (
	"log"
	"os"

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

	watchDir := getEnv("WATCH_DIR", "./watch")
	registryURL := getEnv("REGISTRY_URL", "http://localhost:9000")

	log.Printf("[Agent1] Registry URL：%s", registryURL)
	log.Printf("[Agent1] 監控目錄：%s", watchDir)

	registry := NewRegistryClient(registryURL)

	if err := StartWatcher(watchDir, registry); err != nil {
		log.Fatalf("[Agent1] 監控錯誤：%v", err)
	}
}
