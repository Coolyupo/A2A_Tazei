package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func serveAgentCard(w http.ResponseWriter, r *http.Request) {
	card := AgentCard{
		Name:        "CriticalAlertAnalyzerAgent",
		Description: "接收 Alertmanager Critical 告警，使用 Gemini 進行深度根因分析",
		URL:         getEnv("SELF_URL", "http://localhost:8080"),
		Version:     "3.0.0",
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

// handleRPC 是 A2A 協議的主要入口，接收 JSON-RPC 2.0 請求
func handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "", -32700, "Parse error: "+err.Error())
		return
	}

	log.Printf("[Agent2] 收到請求 | Method: %s | ID: %s", req.Method, req.ID)

	switch req.Method {
	case "tasks/send":
		handleTaskSend(w, req)
	default:
		writeError(w, req.ID, -32601, "Method not found: "+req.Method)
	}
}

func handleTaskSend(w http.ResponseWriter, req JSONRPCRequest) {
	// 將 interface{} 的 Params 反序列化為 Task
	paramBytes, _ := json.Marshal(req.Params)
	var task Task
	if err := json.Unmarshal(paramBytes, &task); err != nil {
		writeError(w, req.ID, -32600, "Invalid task format: "+err.Error())
		return
	}

	log.Printf("[Agent2] 開始處理 Task | ID: %s | Session: %s", task.ID, task.SessionID)
	task.Status = TaskStatus{State: "working", Timestamp: time.Now().Format(time.RFC3339)}

	alertContent := extractTextContent(task.Messages)
	log.Printf("[Agent2] 正在分析 Critical 告警 (%d bytes)", len(alertContent))

	analysis, err := analyzeWithGemini(alertContent)
	if err != nil {
		log.Printf("[Agent2] 分析失敗：%v", err)
		task.Status = TaskStatus{State: "failed", Timestamp: time.Now().Format(time.RFC3339)}
		writeResponse(w, req.ID, task)
		return
	}

	// 將 Gemini 分析結果封裝為 Artifact 回傳
	task.Artifacts = []Artifact{
		{
			Name:  "analysis_report",
			Index: 0,
			Parts: []Part{{Type: "text", Text: analysis}},
		},
	}
	task.Status = TaskStatus{State: "completed", Timestamp: time.Now().Format(time.RFC3339)}
	log.Printf("[Agent2] Task %s 已完成", task.ID)

	writeResponse(w, req.ID, task)
}

func extractTextContent(messages []Message) string {
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type == "text" && part.Text != "" {
				return part.Text
			}
		}
	}
	return ""
}

func writeResponse(w http.ResponseWriter, id string, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id string, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	})
}
