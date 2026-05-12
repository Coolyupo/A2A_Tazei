# A2A Multi-Agent System

以 [A2A（Agent-to-Agent）協議](https://google.github.io/A2A/) 為基礎的多 Agent 系統，透過中央 Registry 做動態服務發現與路由。

Agent 1 持續輪詢 Alertmanager，根據告警的 Severity 自動分流：**Critical** 交給 Agent 2 進行緊急根因分析，**Warning** 交給 Agent 3 進行 Agentic 趨勢評估與自主決策。Agent 3 可自主 Silence 低優先告警（48小時），或將重要告警升級回 Agent 1 由 Slack 通知。

---

## 系統架構

```
                    ┌─────────────────────────────┐
                    │         Registry             │
                    │      localhost:9000          │
                    │                              │
                    │  POST /agents/register       │
                    │  POST /agents/heartbeat      │
                    │  POST /agents/deregister     │
                    │  GET  /agents?skill=xxx      │
                    └──────────┬──────────┬────────┘
                               │注冊/心跳  │注冊/心跳
                    ┌──────────┘          └──────────┐
                    ▼                                 ▼
          ┌─────────────────┐              ┌─────────────────────────────┐
          │    Agent 2      │              │         Agent 3             │
          │ localhost:8080  │              │      localhost:8081         │
          │                 │              │                             │
          │ skill:          │              │ skill: analyze_warning      │
          │ analyze_critical│              │                             │
          │                 │              │  ┌─── Gemini 分析 ───────┐  │
          │ Critical 根因   │              │  │  判斷重要性           │  │
          │ 分析 + 緊急處置 │              │  │                       │  │
          │ Gemini Flash    │              │  ├── 不重要 ─────────────┤  │
          └────────▲────────┘              │  │  POST /api/v2/silences│  │
                   │                       │  │  → Alertmanager 48h   │  │
                   │                       │  │    Silence            │  │
                   │                       │  ├── 重要 ───────────────┤  │
                   │                       │  │  action=escalate      │  │
                   │                       │  │  → 回傳 Agent 1       │  │
                   │                       │  └───────────────────────┘  │
                   │                       └──────────────▲──────────────┘
                   │                                      │
                   │     A2A JSON-RPC tasks/send          │  severity=warning
                   │  severity=critical                   │
                   │                                      │
          ┌────────┴──────────────────────────────────────┴────────┐
          │                      Agent 1                            │
          │          輪詢 Alertmanager（每 30 秒）                  │
          │                                                         │
          │  GET localhost:9093/api/v2/alerts                       │
          │  fingerprint 去重，只處理新進告警                       │
          │                                                         │
          │  severity=critical → analyze_critical → Agent 2         │
          │    └─ 分析完成 → 自動送 Slack                          │
          │                                                         │
          │  severity=warning → analyze_warning → Agent 3           │
          │    ├─ action=silence → 記錄 Silence ID，不通知         │
          │    └─ action=escalate → 送 Slack                       │
          │                                                         │
          │  其他 severity → 忽略                                  │
          └─────────────────────────┬───────────────────────────────┘
                                    │ Slack Incoming Webhook
                                    ▼
                         ┌──────────────────────┐
                         │    Slack Channel      │
                         │  Critical 告警分析    │
                         │  Warning 升級通知     │
                         └──────────────────────┘
                              ▲
                              │ POST /api/v2/alerts
                   ┌──────────┴──────────┐
                   │    Alertmanager      │
                   │  localhost:9093      │
                   └─────────────────────┘
```

### 元件說明

| 元件 | 埠號 | 職責 |
|------|------|------|
| **Registry** | 9000 | Agent 注冊中心，維護可用 Agent 清單，45 秒心跳 TTL |
| **Agent 1** | — | 輪詢 Alertmanager，依 Severity 路由 Task；收到分析結果後送 Slack |
| **Agent 2** | 8080 | 接收 Critical 告警，用 Gemini 進行根因分析與緊急處置建議 |
| **Agent 3** | 8081 | 接收 Warning 告警，Agentic 決策：自動 Silence 或升級給 Agent 1 |
| **Alertmanager** | 9093 | 告警來源（需自行部署，或用 `make test-critical/test-warning` 注入） |
| **Slack** | — | 接收 Agent 1 的分析通知（Critical + Warning 升級） |

### Severity 路由規則

| Severity | 路由至 | Skill ID | Agent 行為 | Slack 通知 |
|----------|--------|----------|------------|------------|
| `critical` | Agent 2 | `analyze_critical` | 根因分析、影響評估、緊急處置 | 一律通知 |
| `warning` | Agent 3 | `analyze_warning` | Agentic 決策（silence / escalate） | 僅 escalate 時通知 |
| 其他 | 忽略 | — | — | — |

### Agent 3 Agentic 決策流程

```
收到 Warning 告警
       │
       ▼
  Gemini 分析告警內容
  並做出自主決策
       │
   ┌───┴───┐
   │       │
silence  escalate
   │       │
   ▼       ▼
呼叫    設定 action=escalate
Alertmanager  回傳 Agent 1
Silence API
（48小時）
```

**決策標準**：
- `silence`：短暫性、已知問題、維護窗口內、或預計 48 小時內自動恢復
- `escalate`：可能演變為 Critical、影響服務可用性、或需要工程師立即確認

---

## 快速開始

### 0. 啟動 Alertmanager

```bash
docker run -d --name alertmanager -p 9093:9093 prom/alertmanager
```

確認啟動成功：

```bash
curl http://localhost:9093/api/v2/status
```

### 1. 安裝依賴

```bash
make setup
```

### 2. 建立 .env 設定檔

```bash
make init-env
```

填入 Gemini API Key（agent2 和 agent3 各需一份），以及 Slack Webhook URL（agent1）：

```bash
echo "GEMINI_API_KEY=你的key" >> agent2/.env
echo "GEMINI_API_KEY=你的key" >> agent3/.env
echo "SLACK_WEBHOOK_URL=https://hooks.slack.com/services/..." >> agent1/.env
```

> Slack Webhook 未設定時，Agent 1 仍會正常運作，分析結果只會輸出到 log。

### 3. 啟動系統（需四個終端機視窗）

**依序啟動，Registry 必須最先啟動：**

```bash
# 終端機 1：Registry（最先啟動）
make run-registry

# 終端機 2：Agent 2（Critical 告警分析）
make run-agent2

# 終端機 3：Agent 3（Warning 告警 Agentic 決策）
make run-agent3

# 終端機 4：Agent 1（Alertmanager 監控 + Slack 通知，最後啟動）
make run-agent1
```

### 4. 測試

> 需要 Alertmanager 在 `localhost:9093` 運行。

```bash
# 送出 Critical 測試告警（觸發 Agent 2，結果送 Slack）
make test-critical

# 送出 Warning 測試告警（觸發 Agent 3，由 AI 決定 silence 或升級 Slack）
make test-warning

# 同時送出兩筆測試告警
make test-all

# 手動送出自定義告警
curl -X POST http://localhost:9093/api/v2/alerts \
  -H "Content-Type: application/json" \
  -d '[{"labels":{"alertname":"DiskFull","severity":"critical","instance":"db-01"},"annotations":{"summary":"磁碟空間不足"}}]'
```

Agent 1 會在下一次輪詢（預設 30 秒）時自動偵測並分派告警。

---

## Registry API

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/agents` | 列出所有已注冊 Agent |
| `GET` | `/agents?skill=analyze_critical` | 依 skill 篩選 Agent |
| `POST` | `/agents/register` | 注冊 Agent（body: AgentCard JSON）|
| `POST` | `/agents/heartbeat` | 送出心跳（body: `{"url":"..."}`）|
| `POST` | `/agents/deregister` | 主動下線（body: `{"url":"..."}`）|

查看已注冊的 Agent：

```bash
curl http://localhost:9000/agents | jq .
curl "http://localhost:9000/agents?skill=analyze_critical" | jq .
curl "http://localhost:9000/agents?skill=analyze_warning" | jq .
```

---

## 環境變數

### Agent 1

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `ALERTMANAGER_URL` | `http://localhost:9093` | Alertmanager 位址 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `POLL_INTERVAL` | `30s` | 輪詢間隔（支援 `10s`、`1m` 等格式） |
| `SLACK_WEBHOOK_URL` | （選填）| Slack Incoming Webhook URL，未設定則不發通知 |

### Agent 2

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `GEMINI_API_KEY` | （必填）| Gemini API 金鑰 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `SELF_URL` | `http://localhost:8080` | 自身對外 URL（用於注冊） |
| `PORT` | `8080` | 監聽埠號 |

### Agent 3

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `GEMINI_API_KEY` | （必填）| Gemini API 金鑰 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `SELF_URL` | `http://localhost:8081` | 自身對外 URL（用於注冊） |
| `PORT` | `8081` | 監聽埠號 |
| `ALERTMANAGER_URL` | `http://localhost:9093` | Alertmanager 位址（用於建立 Silence） |

---

## 目錄結構

```
A2A_TaZei/
├── registry/              # Agent Registry 服務
│   ├── main.go            # HTTP Server，提供注冊/心跳/查詢 API
│   ├── registry.go        # 注冊、心跳、TTL 過期邏輯
│   ├── types.go
│   └── go.mod
├── agent1/                # Alertmanager 監控 + Slack 通知 Agent
│   ├── main.go            # 讀取環境變數，啟動 Poller
│   ├── alertmanager.go    # 輪詢 Alertmanager，依 severity 路由，檢查 Agent3 決策送 Slack
│   ├── slack.go           # Slack Incoming Webhook 訊息發送
│   ├── a2a_client.go      # RegistryClient + A2AClient（附帶 alert metadata）
│   ├── types.go           # AlertmanagerAlert + A2A 協議型別（含 Task.Metadata）
│   └── go.mod
├── agent2/                # Critical 告警分析 Agent
│   ├── main.go
│   ├── server.go          # A2A JSON-RPC server（skill: analyze_critical）
│   ├── gemini.go          # Gemini 根因分析 prompt
│   ├── register.go        # 注冊與心跳邏輯
│   ├── types.go
│   └── go.mod
├── agent3/                # Warning 告警 Agentic 決策 Agent
│   ├── main.go
│   ├── server.go          # A2A JSON-RPC server（skill: analyze_warning）+ agentic 決策
│   ├── gemini.go          # Gemini 分析 + 決策（silence / escalate）
│   ├── alertmanager.go    # Alertmanager Silence API 呼叫
│   ├── register.go        # 注冊與心跳邏輯
│   ├── types.go           # A2A 型別（含 AnalysisResult、Task.Metadata）
│   └── go.mod
└── Makefile
```

---

## A2A 核心概念實現

| A2A 概念 | 本系統實現方式 |
|----------|--------------|
| **Agent Card** | `GET /.well-known/agent.json`，描述 Agent 能力與 Skill |
| **Agent Registry** | 獨立服務（port 9000），Agent 啟動時注冊，每 15 秒送心跳 |
| **Task** | `Task` struct，含 ID、SessionID、Messages、Artifacts、Metadata |
| **Task 生命週期** | submitted → working → completed / failed |
| **Transport** | HTTP POST `/`，JSON-RPC 2.0，Method: `tasks/send` |
| **動態路由** | Agent 1 依 Severity 查 Registry（`analyze_critical` / `analyze_warning`），由 Registry 回傳可用 Agent URL |
| **心跳 TTL** | 45 秒未收到心跳則自動移除，保持 Registry 資料新鮮 |
| **去重機制** | Agent 1 以 fingerprint 為 key 快取已處理告警，告警解除後自動清除快取 |
| **Agentic 決策** | Agent 3 自主判斷 Warning 告警重要性，autonomously 呼叫 Alertmanager Silence API 或升級給 Agent 1 |
| **Task Metadata** | Agent 1 在 Task 中附帶 `fingerprint` 與 `labels` JSON；Agent 3 回傳 `action`、`reason`、`silence_id` |
| **外部通知** | Agent 1 透過 Slack Incoming Webhook 推送 Critical 分析與 Warning 升級報告 |
