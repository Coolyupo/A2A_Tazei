package main

// AlertmanagerAlert 代表從 Alertmanager API 取得的單一告警
type AlertmanagerAlert struct {
	Fingerprint string            `json:"fingerprint"`
	Status      AlertStatus       `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

type AlertStatus struct {
	State string `json:"state"`
}

// AgentCard 用於解析從 Agent 2 / Agent 3 /.well-known/agent.json 取得的能力描述
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

// Task 是 A2A 協議中 Agent 間傳遞的工作單元
type Task struct {
	ID        string     `json:"id"`
	SessionID string     `json:"sessionId"`
	Status    TaskStatus `json:"status"`
	Messages  []Message  `json:"messages,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// TaskStatus 追蹤 Task 生命週期：submitted -> working -> completed/failed
type TaskStatus struct {
	State     string `json:"state"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Message 代表對話中的一則訊息，role 為 "user" 或 "agent"
type Message struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

// Part 是訊息的內容單元，支援 text 與 file 兩種類型
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

// Artifact 是 Agent 執行完成後產出的結果
type Artifact struct {
	Name  string `json:"name"`
	Parts []Part `json:"parts"`
	Index int    `json:"index"`
}

// JSON-RPC 2.0 傳輸層結構
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
