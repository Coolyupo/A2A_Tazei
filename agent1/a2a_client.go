package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)


// RegistryClient 向 Registry 查詢可用的 Agent
type RegistryClient struct {
	RegistryURL string
	httpClient  *http.Client
}

func NewRegistryClient(registryURL string) *RegistryClient {
	return &RegistryClient{
		RegistryURL: registryURL,
		httpClient:  &http.Client{},
	}
}

// FindAgent 根據 skill ID 向 Registry 查詢並回傳第一個可用的 Agent URL
func (rc *RegistryClient) FindAgent(skill string) (string, error) {
	resp, err := rc.httpClient.Get(fmt.Sprintf("%s/agents?skill=%s", rc.RegistryURL, skill))
	if err != nil {
		return "", fmt.Errorf("查詢 Registry 失敗：%w", err)
	}
	defer resp.Body.Close()

	var cards []AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&cards); err != nil {
		return "", fmt.Errorf("解析 Registry 回應失敗：%w", err)
	}

	if len(cards) == 0 {
		return "", fmt.Errorf("找不到支援 %q 的 Agent", skill)
	}

	log.Printf("[Agent1] Registry 回傳 %d 個支援 %q 的 Agent，使用：%s", len(cards), skill, cards[0].URL)
	return cards[0].URL, nil
}

// A2AClient 與指定 Agent 建立 A2A 連線並送出 Task
type A2AClient struct {
	BaseURL    string
	SessionID  string
	httpClient *http.Client
}

func NewA2AClient(baseURL string) *A2AClient {
	return &A2AClient{
		BaseURL:    baseURL,
		SessionID:  uuid.New().String(),
		httpClient: &http.Client{},
	}
}

// SendAlertTask 將 Alertmanager 告警封裝為 A2A Task，透過 JSON-RPC tasks/send 送給目標 Agent
func (c *A2AClient) SendAlertTask(alert AlertmanagerAlert, content string) (*Task, error) {
	taskID := uuid.New().String()
	alertName := alert.Labels["alertname"]
	severity := alert.Labels["severity"]

	labelsJSON, _ := json.Marshal(alert.Labels)

	task := Task{
		ID:        taskID,
		SessionID: c.SessionID,
		Status:    TaskStatus{State: "submitted"},
		Messages: []Message{
			{
				Role: "user",
				Parts: []Part{
					{
						Type: "text",
						Text: content,
					},
				},
			},
		},
		Metadata: map[string]string{
			"fingerprint": alert.Fingerprint,
			"labels":      string(labelsJSON),
		},
	}

	body, err := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      taskID,
		Method:  "tasks/send",
		Params:  task,
	})
	if err != nil {
		return nil, fmt.Errorf("序列化 Task 失敗：%w", err)
	}

	log.Printf("[Agent1] 送出 Task | ID: %s | 告警: %s (%s) | Agent: %s", taskID, alertName, severity, c.BaseURL)

	resp, err := c.httpClient.Post(c.BaseURL+"/", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("HTTP POST 失敗：%w", err)
	}
	defer resp.Body.Close()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("解析回應失敗：%w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC 錯誤 [%d]：%s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	resultBytes, _ := json.Marshal(rpcResp.Result)
	var resultTask Task
	if err := json.Unmarshal(resultBytes, &resultTask); err != nil {
		return nil, fmt.Errorf("解析 Task 結果失敗：%w", err)
	}

	return &resultTask, nil
}
