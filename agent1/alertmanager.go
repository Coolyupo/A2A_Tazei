package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func StartAlertmanagerPoller(alertmanagerURL string, pollInterval time.Duration, registry *RegistryClient) error {
	seen := make(map[string]bool)

	log.Printf("[Agent1] 開始監控 Alertmanager：%s（每 %.0f 秒輪詢一次）", alertmanagerURL, pollInterval.Seconds())

	pollAlertmanager(alertmanagerURL, seen, registry)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		pollAlertmanager(alertmanagerURL, seen, registry)
	}
	return nil
}

func pollAlertmanager(alertmanagerURL string, seen map[string]bool, registry *RegistryClient) {
	url := strings.TrimRight(alertmanagerURL, "/") + "/api/v2/alerts"
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[Agent1] 無法連線到 Alertmanager：%v", err)
		return
	}
	defer resp.Body.Close()

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
		go routeAlert(alert, registry)
	}

	if newCount > 0 {
		log.Printf("[Agent1] 發現 %d 筆新告警", newCount)
	}

	// 清除已解除告警的快取，讓同一告警重新觸發時可被再次處理
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

func routeAlert(alert AlertmanagerAlert, registry *RegistryClient) {
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
	result, err := client.SendAlertTask(alert, content)
	if err != nil {
		log.Printf("[Agent1] Task 執行失敗：%v", err)
		return
	}

	printResult(result)
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

func printResult(task *Task) {
	log.Println("[Agent1] ================================================")
	log.Printf("[Agent1] Task ID : %s", task.ID)
	log.Printf("[Agent1] Session : %s", task.SessionID)
	log.Printf("[Agent1] 狀態    : %s", task.Status.State)
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
