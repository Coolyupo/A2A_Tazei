package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

const approvalTimeout = 10 * time.Minute

// ApprovalResult 是 Socket Mode 核准後回傳給等待中的 goroutine 的結果
type ApprovalResult struct {
	Approved bool
	Task     *Task
	Error    error
}

// PendingApproval 儲存等待 Slack 核准的任務資訊
type PendingApproval struct {
	TaskID     string
	AgentURL   string
	Alert      AlertmanagerAlert
	ToolChoice ToolChoiceSummary
	ResultCh   chan *ApprovalResult
	ExpiresAt  time.Time
	SlackMsgTS string // 用於核准過期後更新訊息
	ChannelID  string
}

// ToolChoiceSummary 是 Agent 2 回傳的工具選擇摘要（從 Task.Metadata 解析）
type ToolChoiceSummary struct {
	Tool        string
	Source      string
	Reason      string
	Description string
	Args        map[string]string
}

// ApprovalManager 管理所有等待核准的任務
type ApprovalManager struct {
	mu      sync.Mutex
	pending map[string]*PendingApproval // key: taskID
}

func NewApprovalManager() *ApprovalManager {
	return &ApprovalManager{
		pending: make(map[string]*PendingApproval),
	}
}

func (am *ApprovalManager) Register(pa *PendingApproval) {
	am.mu.Lock()
	am.pending[pa.TaskID] = pa
	am.mu.Unlock()
}

func (am *ApprovalManager) Pop(taskID string) (*PendingApproval, bool) {
	am.mu.Lock()
	pa, ok := am.pending[taskID]
	if ok {
		delete(am.pending, taskID)
	}
	am.mu.Unlock()
	return pa, ok
}

// SlackBot 封裝 Slack API Client 與 Socket Mode Client
type SlackBot struct {
	api      *slack.Client
	sm       *socketmode.Client
	am       *ApprovalManager
	a2aConns map[string]*A2AClient // agentURL → client（複用 session）
	a2aMu    sync.Mutex
}

func NewSlackBot(botToken, appToken string, am *ApprovalManager) *SlackBot {
	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
	sm := socketmode.New(api, socketmode.OptionDebug(false))
	return &SlackBot{api: api, sm: sm, am: am, a2aConns: make(map[string]*A2AClient)}
}

// Start 在背景啟動 Socket Mode 監聽
func (b *SlackBot) Start() {
	go b.handleEvents()
	go func() {
		if err := b.sm.Run(); err != nil {
			log.Printf("[Agent1/SlackBot] Socket Mode 錯誤：%v", err)
		}
	}()
	log.Printf("[Agent1/SlackBot] Socket Mode 已啟動")
}

func (b *SlackBot) handleEvents() {
	for evt := range b.sm.Events {
		switch evt.Type {
		case socketmode.EventTypeInteractive:
			interaction, ok := evt.Data.(slack.InteractionCallback)
			if !ok {
				b.sm.Ack(*evt.Request)
				continue
			}
			b.sm.Ack(*evt.Request)
			go b.handleInteraction(interaction)
		}
	}
}

func (b *SlackBot) handleInteraction(interaction slack.InteractionCallback) {
	if interaction.Type != slack.InteractionTypeBlockActions {
		return
	}
	for _, action := range interaction.ActionCallback.BlockActions {
		if action.ActionID == "approve_task" {
			b.processApproval(action.Value, interaction.Channel.ID, interaction.Message.Timestamp)
		}
	}
}

func (b *SlackBot) processApproval(taskID, channelID, msgTS string) {
	pa, ok := b.am.Pop(taskID)
	if !ok {
		log.Printf("[Agent1/SlackBot] 找不到 pending approval：%s（可能已過期）", taskID)
		b.updateMessage(channelID, msgTS, "⏰ 此核准請求已過期或已被處理", "#888888")
		return
	}

	if time.Now().After(pa.ExpiresAt) {
		log.Printf("[Agent1/SlackBot] 核准請求已逾時：%s", taskID)
		pa.ResultCh <- &ApprovalResult{Approved: false, Error: fmt.Errorf("審核逾時")}
		b.updateMessage(channelID, msgTS, "⏰ 審核時間已超過 10 分鐘，請求已過期", "#888888")
		return
	}

	log.Printf("[Agent1/SlackBot] 收到核准：Task %s", taskID)
	b.updateMessage(channelID, msgTS, "✅ 已核准，正在執行工具...", "#36a64f")

	// 呼叫 Agent 2 Phase 2
	b.a2aMu.Lock()
	client, exists := b.a2aConns[pa.AgentURL]
	if !exists {
		client = NewA2AClient(pa.AgentURL)
		b.a2aConns[pa.AgentURL] = client
	}
	b.a2aMu.Unlock()

	resultTask, err := client.ApproveTask(taskID)
	if err != nil {
		log.Printf("[Agent1/SlackBot] Agent 2 執行失敗：%v", err)
		pa.ResultCh <- &ApprovalResult{Approved: true, Error: err}
		return
	}

	pa.ResultCh <- &ApprovalResult{Approved: true, Task: resultTask}
}

func (b *SlackBot) updateMessage(channelID, ts, statusText, color string) {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", statusText, false, false),
			nil, nil,
		),
	}
	_, _, _, err := b.api.UpdateMessage(channelID, ts, slack.MsgOptionBlocks(blocks...))
	if err != nil {
		log.Printf("[Agent1/SlackBot] 更新 Slack 訊息失敗：%v", err)
	}
}

// PostApprovalRequest 發送帶 Approve 按鈕的 Slack 訊息，回傳訊息 ts
func (b *SlackBot) PostApprovalRequest(channelID string, alert AlertmanagerAlert, taskID string, choice ToolChoiceSummary) (string, error) {
	alertName := alert.Labels["alertname"]
	instance := alert.Labels["instance"]
	if instance == "" {
		instance = "unknown"
	}

	argsText := "（無）"
	if len(choice.Args) > 0 {
		argsJSON, _ := json.MarshalIndent(choice.Args, "  ", "  ")
		argsText = "```\n" + string(argsJSON) + "\n```"
	}

	sourceLabel := "MCP 工具"
	if choice.Source == "builtin" {
		sourceLabel = "內建分析"
	}

	headerText := fmt.Sprintf(":rotating_light: *Critical 告警工具核准請求*\n告警：*%s* | 主機：`%s`", alertName, instance)
	detailText := fmt.Sprintf("*選定工具*：`%s`（%s）\n*工具說明*：%s\n*AI 選擇原因*：%s\n*執行參數*：%s",
		choice.Tool, sourceLabel, choice.Description, choice.Reason, argsText)
	expiryText := fmt.Sprintf("_此請求將於 %s 後過期_", approvalTimeout.String())

	blocks := []slack.Block{
		slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", "⚠️ 需要人工核准", false, false)),
		slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", headerText, false, false), nil, nil),
		slack.NewDividerBlock(),
		slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", detailText, false, false), nil, nil),
		slack.NewDividerBlock(),
		slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", expiryText, false, false), nil, nil),
		slack.NewActionBlock(
			"approval_actions",
			slack.NewButtonBlockElement(
				"approve_task",
				taskID,
				slack.NewTextBlockObject("plain_text", "✅ Approve", false, false),
			).WithStyle(slack.StylePrimary),
		),
	}

	_, ts, err := b.api.PostMessage(channelID, slack.MsgOptionBlocks(blocks...))
	if err != nil {
		return "", fmt.Errorf("發送 Slack 核准請求失敗：%w", err)
	}
	return ts, nil
}
