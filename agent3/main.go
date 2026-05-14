package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if os.Getenv("GEMINI_API_KEY") == "" {
		log.Fatal("[Agent3] 缺少必要的環境變數：GEMINI_API_KEY")
	}

	selfURL := getEnv("SELF_URL", "http://localhost:8081")
	registryURL := getEnv("REGISTRY_URL", "http://localhost:9000")
	port := getEnv("PORT", "8081")

	startRegistration(selfURL, registryURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", serveAgentCard)
	mux.HandleFunc("/", handleRPC)

	addr := ":" + port
	log.Printf("[Agent3] WarningAlertAnalyzerAgent 啟動於 %s", addr)
	log.Printf("[Agent3] Agent Card: http://localhost%s/.well-known/agent.json", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[Agent3] 伺服器錯誤：%v", err)
	}
}
