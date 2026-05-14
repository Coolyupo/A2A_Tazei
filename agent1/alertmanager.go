package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func StartAlertmanagerPoller(alertmanagerURL string, pollInterval time.Duration, registry *RegistryClient, slackWebhook string, bot *SlackBot, am *ApprovalManager, channelID string) error {
	seen := make(map[string]bool)

	log.Printf("[Agent1] 開始監控 Alertmanager：%s（每 %.0f 秒輪詢一次）", alertmanagerURL, pollInterval.Seconds())

	pollAlertmanager(alertmanagerURL, seen, registry, slackWebhook, bot, am, channelID)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		pollAlertmanager(alertmanagerURL, seen, registry, slackWebhook, bot, am, channelID)
	}
	return nil
}

var amHTTPClient = &http.Client{Timeout: 15 * time.Second}

func pollAlertmanager(alertmanagerURL string, seen map[string]bool, registry *RegistryClient, slackWebhook string, bot *SlackBot, am *ApprovalManager, channelID string) {
	url := strings.TrimRight(alertmanagerURL, "/") + "/api/v2/alerts"
	resp, err := amHTTPClient.Get(url)
	if err != nil {
		log.Printf("[Agent1] 無法連線到 Alertmanager：%v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Agent1] Alertmanager 回傳 HTTP %d", resp.StatusCode)
		return
	}

	var alerts []AlertmanagerAlert
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		log.Printf("[Agent1] 解析 Alertmanager 回應失敗：%v", err)
		return
	}

	newCount := 0
	for _, alert := range alerts {
		if alert.Status.State != "active" {
			continue
		}
		if seen[alert.Fingerprint] {
			continue
		}
		seen[alert.Fingerprint] = true
		newCount++
		go routeAlert(alert, registry, slackWebhook, bot, am, channelID)
	}

	if newCount > 0 {
		log.Printf("[Agent1] 發現 %d 筆新告警", newCount)
	}

	active := make(map[string]bool)
	for _, alert := range alerts {
		if alert.Status.State == "active" {
			active[alert.Fingerprint] = true
		}
	}
	for fp := range seen {
		if !active[fp] {
			delete(seen, fp)
		}
	}
}

func routeAlert(alert AlertmanagerAlert, registry *RegistryClient, slackWebhook string, bot *SlackBot, am *ApprovalManager, channelID string) {
	severity := strings.ToLower(alert.Labels["severity"])
	alertName := alert.Labels["alertname"]

	var skill string
	switch severity {
	case "critical":
		skill = "analyze_critical"
	case "warning":
		skill = "analyze_warning"
	default:
		log.Printf("[Agent1] 忽略告警 %s（severity=%q，不在路由規則內）", alertName, severity)
		return
	}

	agentURL, err := registry.FindAgent(skill)
	if err != nil {
		log.Printf("[Agent1] 找不到可用的 Agent（skill: %s）：%v", skill, err)
		return
	}

	content := formatAlertContent(alert)
	client := NewA2AClient(agentURL)

	switch severity {
	case "critical":
		routeCritical(alert, content, client, agentURL, slackWebhook, bot, am, channelID)
	case "warning":
		routeWarning(alert, content, client, slackWebhook)
	}
}

// routeCritical 兩階段流程：工具選擇 → Slack 核准 → 執行 → 結果通知
func routeCritical(alert AlertmanagerAlert, content string, client *A2AClient, agentURL string, slackWebhook string, bot *SlackBot, am *ApprovalManager, channelID string) {
	alertName := alert.Labels["alertname"]

	// Phase 1：送到 Agent 2，取得工具選擇
	phase1Result, err := client.SendAlertTask(alert, content)
	if err != nil {
		log.Printf("[Agent1] Phase 1 失敗（%s）：%v", alertName, err)
		return
	}

	printResult(phase1Result, "Phase 1")

	if phase1Result.Status.State != "awaiting_approval" {
		// Agent 2 直接完成（罕見情況），送 Slack
		log.Printf("[Agent1] Agent 2 直接完成（state: %s），送 Slack", phase1Result.Status.State)
		sendAlertToSlack(slackWebhook, alert, phase1Result, "critical")
		return
	}

	// 解析工具選擇
	choice := parseToolChoice(phase1Result.Metadata)
	log.Printf("[Agent1] 工具選擇：%s（%s），等待 Slack 核准", choice.Tool, choice.Source)

	if bot == nil || channelID == "" {
		log.Printf("[Agent1] Slack Bot 未設定，自動核准 Task %s", phase1Result.ID)
		autoApproveAndNotify(client, phase1Result.ID, alert, slackWebhook)
		return
	}

	// 發送 Slack 核准請求
	ts, err := bot.PostApprovalRequest(channelID, alert, phase1Result.ID, choice)
	if err != nil {
		log.Printf("[Agent1] 發送 Slack 核准請求失敗：%v，改為自動核准", err)
		autoApproveAndNotify(client, phase1Result.ID, alert, slackWebhook)
		return
	}

	// 註冊到 ApprovalManager，等待 Slack 回應
	resultCh := make(chan *ApprovalResult, 1)
	pa := &PendingApproval{
		TaskID:     phase1Result.ID,
		AgentURL:   agentURL,
		Alert:      alert,
		ToolChoice: choice,
		ResultCh:   resultCh,
		ExpiresAt:  time.Now().Add(approvalTimeout),
		SlackMsgTS: ts,
		ChannelID:  channelID,
	}
	am.Register(pa)

	// 等待核准（最多 10 分鐘）
	select {
	case result := <-resultCh:
		if !result.Approved || result.Error != nil {
			msg := "審核逾時或拒絕"
			if result.Error != nil {
				msg = result.Error.Error()
			}
			log.Printf("[Agent1] Task %s 未完成：%s", phase1Result.ID, msg)
			bot.updateMessage(channelID, ts, fmt.Sprintf("⏰ %s", msg), "#888888")
			return
		}

		log.Printf("[Agent1] Task %s 執行完成", phase1Result.ID)
		printResult(result.Task, "Phase 2")
		bot.updateMessage(channelID, ts, fmt.Sprintf("✅ 執行完成：`%s`", choice.Tool), "#36a64f")
		sendAlertToSlack(slackWebhook, alert, result.Task, "critical")

	case <-time.After(approvalTimeout + 30*time.Second):
		// 雙重保險：本地計時
		am.Pop(phase1Result.ID)
		log.Printf("[Agent1] Task %s 核准等待超時（本地計時器）", phase1Result.ID)
		bot.updateMessage(channelID, ts, "⏰ 審核時間已超過 10 分鐘，請求已過期", "#888888")
	}
}

// routeWarning Warning 告警流程（Agent 3 Agentic，無需核准）
func routeWarning(alert AlertmanagerAlert, content string, client *A2AClient, slackWebhook string) {
	result, err := client.SendAlertTask(alert, content)
	if err != nil {
		log.Printf("[Agent1] Warning Task 失敗：%v", err)
		return
	}

	printResult(result, "Warning")

	action := result.Metadata["action"]
	switch action {
	case "escalate":
		log.Printf("[Agent1] Agent3 決策升級，送出 Slack 通知")
		sendAlertToSlack(slackWebhook, alert, result, "warning")
	case "silence":
		log.Printf("[Agent1] Agent3 已自動 Silence 48小時（SilenceID: %s，原因: %s）",
			result.Metadata["silence_id"], result.Metadata["reason"])
	default:
		log.Printf("[Agent1] Agent3 回傳未知 action=%q", action)
	}
}

func autoApproveAndNotify(client *A2AClient, taskID string, alert AlertmanagerAlert, slackWebhook string) {
	result, err := client.ApproveTask(taskID)
	if err != nil {
		log.Printf("[Agent1] 自動核准失敗：%v", err)
		return
	}
	printResult(result, "AutoApprove")
	sendAlertToSlack(slackWebhook, alert, result, "critical")
}

func parseToolChoice(meta map[string]string) ToolChoiceSummary {
	if meta == nil {
		return ToolChoiceSummary{Tool: "builtin", Source: "builtin"}
	}
	var args map[string]string
	if s := meta["toolArgs"]; s != "" {
		json.Unmarshal([]byte(s), &args)
	}
	return ToolChoiceSummary{
		Tool:        meta["tool"],
		Source:      meta["toolSource"],
		Reason:      meta["toolReason"],
		Description: meta["toolDescription"],
		Args:        args,
	}
}

func formatAlertContent(alert AlertmanagerAlert) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("AlertName: %s\n", alert.Labels["alertname"]))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", alert.Labels["severity"]))
	sb.WriteString(fmt.Sprintf("StartsAt: %s\n", alert.StartsAt))
	sb.WriteString(fmt.Sprintf("Fingerprint: %s\n", alert.Fingerprint))
	sb.WriteString("\nLabels:\n")
	for k, v := range alert.Labels {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	if len(alert.Annotations) > 0 {
		sb.WriteString("\nAnnotations:\n")
		for k, v := range alert.Annotations {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}
	return sb.String()
}

func printResult(task *Task, phase string) {
	log.Println("[Agent1] ================================================")
	log.Printf("[Agent1] [%s] Task ID : %s", phase, task.ID)
	log.Printf("[Agent1] 狀態    : %s", task.Status.State)
	if tool := task.Metadata["tool"]; tool != "" {
		log.Printf("[Agent1] 工具    : %s（%s）", tool, task.Metadata["toolSource"])
	}
	if action := task.Metadata["action"]; action != "" {
		log.Printf("[Agent1] AI 決策 : %s（%s）", action, task.Metadata["reason"])
	}
	log.Println("[Agent1] -------- 分析報告 --------")
	if len(task.Artifacts) == 0 {
		log.Println("[Agent1] (無分析結果)")
	} else {
		for _, artifact := range task.Artifacts {
			for _, part := range artifact.Parts {
				if part.Type == "text" {
					log.Println(part.Text)
				}
			}
		}
	}
	log.Println("[Agent1] ================================================")
}
