package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func serveAgentCard(w http.ResponseWriter, r *http.Request) {
	card := AgentCard{
		Name:        "WarningAlertAnalyzerAgent",
		Description: "接收 Alertmanager Warning 告警，使用 Gemini 自主判斷是否 Silence 或升級通知",
		URL:         getEnv("SELF_URL", "http://localhost:8081"),
		Version:     "4.0.0",
		Capabilities: Capabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: []AgentSkill{
			{
				ID:          "analyze_warning",
				Name:        "Warning Alert Analysis",
				Description: "分析 Warning 告警，自主決策：自動 Silence 48小時（不重要）或升級給 Agent 1 通知（重要）",
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

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

	log.Printf("[Agent3] 收到請求 | Method: %s | ID: %s", req.Method, req.ID)

	switch req.Method {
	case "tasks/send":
		handleTaskSend(w, req)
	default:
		writeError(w, req.ID, -32601, "Method not found: "+req.Method)
	}
}

func handleTaskSend(w http.ResponseWriter, req JSONRPCRequest) {
	paramBytes, _ := json.Marshal(req.Params)
	var task Task
	if err := json.Unmarshal(paramBytes, &task); err != nil {
		writeError(w, req.ID, -32600, "Invalid task format: "+err.Error())
		return
	}

	log.Printf("[Agent3] 開始處理 Task | ID: %s | Session: %s", task.ID, task.SessionID)
	task.Status = TaskStatus{State: "working", Timestamp: time.Now().Format(time.RFC3339)}

	alertContent := extractTextContent(task.Messages)
	log.Printf("[Agent3] 正在分析 Warning 告警（agentic 模式，%d bytes）", len(alertContent))

	result, err := analyzeAndDecide(alertContent)
	if err != nil {
		log.Printf("[Agent3] 分析失敗：%v", err)
		task.Status = TaskStatus{State: "failed", Timestamp: time.Now().Format(time.RFC3339)}
		writeResponse(w, req.ID, task)
		return
	}

	log.Printf("[Agent3] AI 決策：%s（原因：%s）", result.Decision, result.Reason)

	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	task.Metadata["action"] = result.Decision
	task.Metadata["reason"] = result.Reason

	if result.Decision == "silence" {
		var labels map[string]string
		if labelsStr := task.Metadata["labels"]; labelsStr != "" {
			json.Unmarshal([]byte(labelsStr), &labels)
		}
		if len(labels) == 0 {
			labels = map[string]string{"alertname": "unknown"}
		}

		silenceID, err := silenceAlert(labels, 48*time.Hour, result.Reason)
		if err != nil {
			log.Printf("[Agent3] 自動 Silence 失敗：%v，改為 escalate", err)
			task.Metadata["action"] = "escalate"
			task.Metadata["silence_error"] = err.Error()
		} else {
			task.Metadata["silence_id"] = silenceID
			log.Printf("[Agent3] 告警已自動 Silence 48小時（SilenceID: %s）", silenceID)
		}
	} else {
		log.Printf("[Agent3] 告警需要升級 → 回傳 Agent1 透過 Slack 通知")
	}

	task.Artifacts = []Artifact{
		{
			Name:  "warning_alert_report",
			Index: 0,
			Parts: []Part{{Type: "text", Text: result.Analysis}},
		},
	}
	task.Status = TaskStatus{State: "completed", Timestamp: time.Now().Format(time.RFC3339)}
	log.Printf("[Agent3] Task %s 已完成（action: %s）", task.ID, task.Metadata["action"])

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
