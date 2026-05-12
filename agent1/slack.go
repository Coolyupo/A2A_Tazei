package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type slackMessage struct {
	Text        string            `json:"text,omitempty"`
	Attachments []slackAttachment `json:"attachments,omitempty"`
}

type slackAttachment struct {
	Color  string       `json:"color"`
	Title  string       `json:"title"`
	Text   string       `json:"text"`
	Fields []slackField `json:"fields,omitempty"`
	Footer string       `json:"footer,omitempty"`
}

type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func sendAlertToSlack(webhookURL string, alert AlertmanagerAlert, result *Task, source string) {
	if webhookURL == "" {
		return
	}

	var color, title string
	switch source {
	case "critical":
		color = "#FF0000"
		title = fmt.Sprintf(":rotating_light: [Critical] %s", alert.Labels["alertname"])
	default:
		color = "#FFA500"
		title = fmt.Sprintf(":warning: [Warning 升級] %s", alert.Labels["alertname"])
	}

	fields := []slackField{
		{Title: "Severity", Value: strings.ToUpper(alert.Labels["severity"]), Short: true},
		{Title: "Fingerprint", Value: alert.Fingerprint, Short: true},
	}
	for k, v := range alert.Labels {
		if k == "severity" || k == "alertname" {
			continue
		}
		fields = append(fields, slackField{Title: k, Value: v, Short: true})
	}

	analysis := extractAnalysis(result)
	if len(analysis) > 2800 {
		analysis = analysis[:2800] + "\n…（截斷）"
	}
	if reason := result.Metadata["reason"]; reason != "" {
		analysis = fmt.Sprintf("*AI 決策原因*：%s\n\n%s", reason, analysis)
	}

	msg := slackMessage{
		Attachments: []slackAttachment{
			{
				Color:  color,
				Title:  title,
				Text:   analysis,
				Fields: fields,
				Footer: fmt.Sprintf("A2A Multi-Agent | Agent: %s | StartsAt: %s", source, alert.StartsAt),
			},
		},
	}

	body, _ := json.Marshal(msg)
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[Agent1] Slack 通知失敗：%v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Agent1] Slack 回傳 HTTP %d", resp.StatusCode)
		return
	}
	log.Printf("[Agent1] Slack 通知已送出：%s（來源：%s）", alert.Labels["alertname"], source)
}

func extractAnalysis(task *Task) string {
	for _, artifact := range task.Artifacts {
		for _, part := range artifact.Parts {
			if part.Type == "text" && part.Text != "" {
				return part.Text
			}
		}
	}
	return "(無分析內容)"
}
