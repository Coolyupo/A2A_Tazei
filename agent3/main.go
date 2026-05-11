package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	selfURL := getEnv("SELF_URL", "http://localhost:8081")
	registryURL := getEnv("REGISTRY_URL", "http://localhost:9000")
	port := getEnv("PORT", "8081")

	startRegistration(selfURL, registryURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", serveAgentCard)
	mux.HandleFunc("/", handleRPC)

	addr := ":" + port
	log.Printf("[Agent3] ImageAnalyzerAgent 啟動於 %s", addr)
	log.Printf("[Agent3] Agent Card: http://localhost%s/.well-known/agent.json", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[Agent3] 伺服器錯誤：%v", err)
	}
}
