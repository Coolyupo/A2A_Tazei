package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	mcpClient *MCPClient // nil if MCP not configured

	pendingMu    sync.RWMutex
	pendingTasks = make(map[string]*PendingTask)
)

// startPendingCleaner 定期清理超過 15 分鐘的待審核任務
func startPendingCleaner() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-15 * time.Minute)
			pendingMu.Lock()
			for id, pt := range pendingTasks {
				ts := pt.Task.Status.Timestamp
				if t, err := time.Parse(time.RFC3339, ts); err == nil && t.Before(cutoff) {
					delete(pendingTasks, id)
					log.Printf("[Agent2] 清除逾期 pending task：%s", id)
				}
			}
			pendingMu.Unlock()
		}
	}()
}

func serveAgentCard(w http.ResponseWriter, r *http.Request) {
	card := AgentCard{
		Name:        "CriticalAlertAnalyzerAgent",
		Description: "接收 Alertmanager Critical 告警，透過 MCP Client 選擇最適工具並等待 Agent 1 核准後執行",
		URL:         getEnv("SELF_URL", "http://localhost:8080"),
		Version:     "4.0.0",
		Capabilities: Capabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: []AgentSkill{
			{
				ID:          "analyze_critical",
				Name:        "Critical Alert Analysis with MCP",
				Description: "分析 Critical 告警，透過 MCP 選擇工具並請求 Agent 1 核准後執行",
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

	log.Printf("[Agent2] 收到請求 | Method: %s | ID: %s", req.Method, req.ID)

	switch req.Method {
	case "tasks/send":
		handleTaskSend(w, req)
	case "tasks/approve":
		handleTaskApprove(w, req)
	default:
		writeError(w, req.ID, -32601, "Method not found: "+req.Method)
	}
}

// handleTaskSend — Phase 1：分析告警，選擇工具，回傳 awaiting_approval
func handleTaskSend(w http.ResponseWriter, req JSONRPCRequest) {
	paramBytes, _ := json.Marshal(req.Params)
	var task Task
	if err := json.Unmarshal(paramBytes, &task); err != nil {
		writeError(w, req.ID, -32600, "Invalid task format: "+err.Error())
		return
	}

	log.Printf("[Agent2] Phase 1 開始 | Task: %s", task.ID)
	task.Status = TaskStatus{State: "working", Timestamp: time.Now().Format(time.RFC3339)}

	alertContent := extractTextContent(task.Messages)

	// 取得 MCP 工具清單（若 MCP 未設定則為空）
	var mcpTools []MCPTool
	if mcpClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		tools, err := mcpClient.ListTools(ctx)
		cancel()
		if err != nil {
			log.Printf("[Agent2] 取得 MCP 工具清單失敗：%v（繼續使用 builtin）", err)
		} else {
			mcpTools = tools
			log.Printf("[Agent2] 取得 %d 個 MCP 工具", len(mcpTools))
		}
	}

	// Gemini 分析並選擇工具
	choice, err := selectToolWithGemini(alertContent, mcpTools)
	if err != nil {
		log.Printf("[Agent2] 工具選擇失敗：%v，改用 builtin", err)
		choice = &ToolChoice{
			Tool:        "builtin",
			Source:      "builtin",
			Args:        map[string]string{},
			Reason:      "工具選擇過程發生錯誤，使用內建分析",
			Description: "使用 Gemini 進行根因分析",
		}
	}

	log.Printf("[Agent2] 工具選擇：%s（來源：%s）原因：%s", choice.Tool, choice.Source, choice.Reason)

	// 儲存待審核任務
	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}
	task.Metadata["tool"] = choice.Tool
	task.Metadata["toolSource"] = choice.Source
	task.Metadata["toolReason"] = choice.Reason
	task.Metadata["toolDescription"] = choice.Description

	argsJSON, _ := json.Marshal(choice.Args)
	task.Metadata["toolArgs"] = string(argsJSON)

	task.Status = TaskStatus{State: "awaiting_approval", Timestamp: time.Now().Format(time.RFC3339)}

	pendingMu.Lock()
	pendingTasks[task.ID] = &PendingTask{
		Task:         task,
		ToolChoice:   *choice,
		AlertContent: alertContent,
	}
	pendingMu.Unlock()

	log.Printf("[Agent2] Task %s 等待 Agent1 核准（工具：%s）", task.ID, choice.Tool)
	writeResponse(w, req.ID, task)
}

// handleTaskApprove — Phase 2：Agent 1 核准後執行工具，回傳結果
func handleTaskApprove(w http.ResponseWriter, req JSONRPCRequest) {
	paramBytes, _ := json.Marshal(req.Params)
	var params struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(paramBytes, &params); err != nil || params.TaskID == "" {
		writeError(w, req.ID, -32600, "Invalid params: taskId required")
		return
	}

	pendingMu.Lock()
	pt, ok := pendingTasks[params.TaskID]
	if ok {
		delete(pendingTasks, params.TaskID)
	}
	pendingMu.Unlock()

	if !ok {
		writeError(w, req.ID, -32602, "Task not found or already executed: "+params.TaskID)
		return
	}

	log.Printf("[Agent2] Phase 2 開始 | Task: %s | 工具：%s（%s）", params.TaskID, pt.ToolChoice.Tool, pt.ToolChoice.Source)

	task := pt.Task
	task.Status = TaskStatus{State: "working", Timestamp: time.Now().Format(time.RFC3339)}

	var result string
	var execErr error

	switch pt.ToolChoice.Source {
	case "mcp":
		if mcpClient == nil {
			log.Printf("[Agent2] MCP Client 未初始化，改用 builtin")
			result, execErr = executeBuiltinAnalysis(pt.AlertContent)
		} else {
			args := make(map[string]interface{})
			for k, v := range pt.ToolChoice.Args {
				args[k] = v
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			result, execErr = mcpClient.CallTool(ctx, pt.ToolChoice.Tool, args)
			cancel()
		}
	default:
		result, execErr = executeBuiltinAnalysis(pt.AlertContent)
	}

	if execErr != nil {
		log.Printf("[Agent2] 工具執行失敗：%v", execErr)
		task.Status = TaskStatus{State: "failed", Timestamp: time.Now().Format(time.RFC3339)}
		task.Metadata["error"] = execErr.Error()
		writeResponse(w, req.ID, task)
		return
	}

	task.Artifacts = []Artifact{
		{
			Name:  "execution_report",
			Index: 0,
			Parts: []Part{{Type: "text", Text: result}},
		},
	}
	task.Status = TaskStatus{State: "completed", Timestamp: time.Now().Format(time.RFC3339)}
	log.Printf("[Agent2] Task %s 執行完成", params.TaskID)

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
