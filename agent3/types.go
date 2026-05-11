package main

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
	ID        string     `json:"id"`
	SessionID string     `json:"sessionId"`
	Status    TaskStatus `json:"status"`
	Messages  []Message  `json:"messages,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
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
