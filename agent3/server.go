package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func serveAgentCard(w http.ResponseWriter, r *http.Request) {
	card := AgentCard{
		Name:        "ImageAnalyzerAgent",
		Description: "接收圖片檔案，使用 Gemini Vision 進行異常分析",
		URL:         getEnv("SELF_URL", "http://localhost:8081"),
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

	filename, base64Content, mimeType := extractImageContent(task.Messages)
	log.Printf("[Agent3] 正在分析圖片：%s (mime: %s)", filename, mimeType)

	analysis, err := analyzeImageWithGemini(filename, base64Content, mimeType)
	if err != nil {
		log.Printf("[Agent3] 分析失敗：%v", err)
		task.Status = TaskStatus{State: "failed", Timestamp: time.Now().Format(time.RFC3339)}
		writeResponse(w, req.ID, task)
		return
	}

	task.Artifacts = []Artifact{
		{
			Name:  "image_analysis_report",
			Index: 0,
			Parts: []Part{{Type: "text", Text: analysis}},
		},
	}
	task.Status = TaskStatus{State: "completed", Timestamp: time.Now().Format(time.RFC3339)}
	log.Printf("[Agent3] Task %s 已完成", task.ID)

	writeResponse(w, req.ID, task)
}

func extractImageContent(messages []Message) (filename, base64Content, mimeType string) {
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type == "file" && part.File != nil {
				return part.File.Name, part.File.Content, part.File.MimeType
			}
		}
	}
	return "unknown.jpg", "", "image/jpeg"
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
