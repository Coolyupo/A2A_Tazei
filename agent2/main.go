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
	mcpServerURL := getEnv("MCP_SERVER_URL", "")

	startRegistration(selfURL, registryURL)

	// 嘗試連線 MCP Server（選填）
	if mcpServerURL != "" {
		c, err := NewMCPClient(mcpServerURL)
		if err != nil {
			log.Printf("[Agent2] MCP Client 初始化失敗：%v（繼續以 builtin 模式運作）", err)
		} else {
			mcpClient = c
		}
	} else {
		log.Printf("[Agent2] MCP_SERVER_URL 未設定，以 builtin 模式運作")
	}

	startPendingCleaner()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", serveAgentCard)
	mux.HandleFunc("/", handleRPC)

	addr := ":" + port
	log.Printf("[Agent2] CriticalAlertAnalyzerAgent 啟動於 %s（MCP: %v）", addr, mcpClient != nil)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[Agent2] 伺服器錯誤：%v", err)
	}
}
