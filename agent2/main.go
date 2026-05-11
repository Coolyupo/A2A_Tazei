package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	selfURL := getEnv("SELF_URL", "http://localhost:8080")
	registryURL := getEnv("REGISTRY_URL", "http://localhost:9000")
	port := getEnv("PORT", "8080")

	startRegistration(selfURL, registryURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", serveAgentCard)
	mux.HandleFunc("/", handleRPC)

	addr := ":" + port
	log.Printf("[Agent2] TextAnalyzerAgent 啟動於 %s", addr)
	log.Printf("[Agent2] Agent Card: http://localhost%s/.well-known/agent.json", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[Agent2] 伺服器錯誤：%v", err)
	}
}
