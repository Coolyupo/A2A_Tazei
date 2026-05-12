package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type mcpResponse struct {
	Result json.RawMessage
	Err    error
}

// MCPClient 透過 SSE 協議連線到 MCP Server，支援 tools/list 與 tools/call
type MCPClient struct {
	serverURL   string
	msgEndpoint string
	httpClient  *http.Client

	mu      sync.Mutex
	pending map[int64]chan *mcpResponse
	idGen   atomic.Int64
}

func NewMCPClient(serverURL string) (*MCPClient, error) {
	c := &MCPClient{
		serverURL:  serverURL,
		httpClient: &http.Client{Timeout: 0}, // SSE 需要長連線，不設 timeout
		pending:    make(map[int64]chan *mcpResponse),
	}

	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *MCPClient) connect() error {
	req, err := http.NewRequest(http.MethodGet, c.serverURL+"/sse", nil)
	if err != nil {
		return fmt.Errorf("建立 SSE 請求失敗：%w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("連線 MCP SSE 失敗：%w", err)
	}

	endpointCh := make(chan string, 1)

	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		var eventType, dataLine string

		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				eventType = strings.TrimSpace(line[6:])
			case strings.HasPrefix(line, "data:"):
				dataLine = strings.TrimSpace(line[5:])
			case line == "":
				c.handleSSEEvent(eventType, dataLine, endpointCh)
				eventType, dataLine = "", ""
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("[Agent2/MCP] SSE 讀取中斷：%v", err)
		}
	}()

	select {
	case endpoint := <-endpointCh:
		if strings.HasPrefix(endpoint, "/") {
			// 相對路徑，拼上 base URL
			base := strings.TrimRight(c.serverURL, "/")
			c.msgEndpoint = base + endpoint
		} else {
			c.msgEndpoint = endpoint
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("等待 MCP endpoint 超時")
	}

	// initialize
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := c.rpc(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "Agent2-MCP-Client", "version": "4.0.0"},
	}); err != nil {
		return fmt.Errorf("MCP initialize 失敗：%w", err)
	}

	// initialized notification（不等回應）
	c.notify("notifications/initialized", nil)

	log.Printf("[Agent2/MCP] 已連線：%s（endpoint: %s）", c.serverURL, c.msgEndpoint)
	return nil
}

func (c *MCPClient) handleSSEEvent(eventType, dataLine string, endpointCh chan<- string) {
	switch eventType {
	case "endpoint":
		select {
		case endpointCh <- dataLine:
		default:
		}
	case "message":
		if dataLine == "" {
			return
		}
		var base struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(dataLine), &base); err != nil {
			return
		}

		c.mu.Lock()
		ch, ok := c.pending[base.ID]
		if ok {
			delete(c.pending, base.ID)
		}
		c.mu.Unlock()

		if ok {
			r := &mcpResponse{Result: base.Result}
			if base.Error != nil {
				r.Err = fmt.Errorf("MCP error %d: %s", base.Error.Code, base.Error.Message)
			}
			ch <- r
		}
	}
}

func (c *MCPClient) rpc(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.idGen.Add(1)

	ch := make(chan *mcpResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	body, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.msgEndpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("POST 到 MCP endpoint 失敗：%w", err)
	}
	resp.Body.Close()

	select {
	case r := <-ch:
		return r.Result, r.Err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("MCP RPC 超時（method: %s）", method)
	}
}

func (c *MCPClient) notify(method string, params interface{}) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}
	body, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(c.msgEndpoint, "application/json", bytes.NewReader(body))
	if err == nil && resp != nil {
		resp.Body.Close()
	}
}

// ListTools 取得 MCP Server 上所有可用工具
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	result, err := c.rpc(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("解析 tools/list 回應失敗：%w", err)
	}
	return resp.Tools, nil
}

// CallTool 執行指定的 MCP 工具
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := c.rpc(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return string(result), nil
	}
	if resp.IsError {
		return "", fmt.Errorf("MCP 工具執行失敗")
	}

	var parts []string
	for _, c := range resp.Content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}
