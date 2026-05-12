package main

import "encoding/json"

type AgentCard struct {
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	URL          string       `json:"url"`
	Version      string       `json:"version"`
	Capabilities Capabilities `json:"capabilities"`
	Skills       []AgentSkill `json:"skills"`
}

type Capabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	InputModes  []string `json:"inputModes"`
	OutputModes []string `json:"outputModes"`
}

type Task struct {
	ID        string            `json:"id"`
	SessionID string            `json:"sessionId"`
	Status    TaskStatus        `json:"status"`
	Messages  []Message         `json:"messages,omitempty"`
	Artifacts []Artifact        `json:"artifacts,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TaskStatus struct {
	State     string `json:"state"`
	Timestamp string `json:"timestamp,omitempty"`
}

type Message struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Type string    `json:"type"`
	Text string    `json:"text,omitempty"`
	File *FilePart `json:"file,omitempty"`
}

type FilePart struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Content  string `json:"content"`
}

type Artifact struct {
	Name  string `json:"name"`
	Parts []Part `json:"parts"`
	Index int    `json:"index"`
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPTool 描述從 MCP Server 發現的工具
type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// ToolChoice 是 Gemini 分析後選定的工具及其執行參數
type ToolChoice struct {
	Tool        string            `json:"tool"`        // 工具名稱或 "builtin"
	Source      string            `json:"source"`      // "mcp" 或 "builtin"
	Args        map[string]string `json:"args"`        // 工具執行參數
	Reason      string            `json:"reason"`      // 選擇理由
	Description string            `json:"description"` // 工具將做什麼
}

// PendingTask 儲存等待 Agent 1 核准的任務
type PendingTask struct {
	Task         Task
	ToolChoice   ToolChoice
	AlertContent string
}
