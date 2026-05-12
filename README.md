# A2A Multi-Agent System

以 [A2A（Agent-to-Agent）協議](https://google.github.io/A2A/) 為基礎的多 Agent 系統，透過中央 Registry 做動態服務發現與路由。

Agent 1 持續輪詢 Alertmanager，根據告警的 Severity 自動分流：**Critical** 交給 Agent 2 進行緊急根因分析，**Warning** 交給 Agent 3 進行趨勢評估與預防建議。

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
          ┌─────────────────┐              ┌─────────────────┐
          │    Agent 2      │              │    Agent 3      │
          │ localhost:8080  │              │ localhost:8081  │
          │                 │              │                 │
          │ skill:          │              │ skill:          │
          │ analyze_critical│              │ analyze_warning │
          │                 │              │                 │
          │ Critical 根因   │              │ Warning 趨勢    │
          │ 分析 + 緊急處置 │              │ 分析 + 預防建議 │
          │ Gemini Flash    │              │ Gemini Flash    │
          └────────▲────────┘              └────────▲────────┘
                   │                                │
                   │     A2A JSON-RPC tasks/send    │
                   │  severity=critical             │  severity=warning
                   │                                │
          ┌────────┴────────────────────────────────┴────────┐
          │                   Agent 1                        │
          │         輪詢 Alertmanager（每 30 秒）             │
          │                                                   │
          │  GET localhost:9093/api/v2/alerts                 │
          │  fingerprint 去重，只處理新進告警                 │
          │                                                   │
          │  severity=critical → analyze_critical → Agent 2  │
          │  severity=warning  → analyze_warning  → Agent 3  │
          │  其他 severity     → 忽略                        │
          └──────────────────────────────────────────────────┘
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
| **Agent 1** | — | 輪詢 Alertmanager，依 Severity 向 Registry 查詢並路由 Task |
| **Agent 2** | 8080 | 接收 Critical 告警，用 Gemini 進行根因分析與緊急處置建議 |
| **Agent 3** | 8081 | 接收 Warning 告警，用 Gemini 進行趨勢分析與預防建議 |
| **Alertmanager** | 9093 | 告警來源（需自行部署，或用 `make test-critical/test-warning` 注入） |

### Severity 路由規則

| Severity | 路由至 | Skill ID | 分析重點 |
|----------|--------|----------|----------|
| `critical` | Agent 2 | `analyze_critical` | 根因分析、影響評估、緊急處置 |
| `warning` | Agent 3 | `analyze_warning` | 趨勢判斷、潛在風險、預防建議 |
| 其他 | 忽略 | — | — |

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

填入 Gemini API Key（agent2 和 agent3 各需一份）：

```bash
echo "GEMINI_API_KEY=你的key" >> agent2/.env
echo "GEMINI_API_KEY=你的key" >> agent3/.env
```

### 3. 啟動系統（需四個終端機視窗）

**依序啟動，Registry 必須最先啟動：**

```bash
# 終端機 1：Registry（最先啟動）
make run-registry

# 終端機 2：Agent 2（Critical 告警分析）
make run-agent2

# 終端機 3：Agent 3（Warning 告警分析）
make run-agent3

# 終端機 4：Agent 1（Alertmanager 監控，最後啟動）
make run-agent1
```

### 4. 測試

> 需要 Alertmanager 在 `localhost:9093` 運行。

```bash
# 送出 Critical 測試告警（觸發 Agent 2）
make test-critical

# 送出 Warning 測試告警（觸發 Agent 3）
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

### Agent 2 / Agent 3

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `GEMINI_API_KEY` | （必填）| Gemini API 金鑰 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `SELF_URL` | `:8080` / `:8081` | 自身對外 URL（用於注冊） |
| `PORT` | `8080` / `8081` | 監聽埠號 |

---

## 目錄結構

```
A2A_TaZei/
├── registry/              # Agent Registry 服務
│   ├── main.go            # HTTP Server，提供注冊/心跳/查詢 API
│   ├── registry.go        # 注冊、心跳、TTL 過期邏輯
│   ├── types.go
│   └── go.mod
├── agent1/                # Alertmanager 監控 Agent
│   ├── main.go            # 讀取 ALERTMANAGER_URL / POLL_INTERVAL
│   ├── alertmanager.go    # 輪詢 Alertmanager，依 severity 路由 Task
│   ├── a2a_client.go      # RegistryClient + A2AClient
│   ├── types.go           # AlertmanagerAlert + A2A 協議型別
│   └── go.mod
├── agent2/                # Critical 告警分析 Agent
│   ├── main.go
│   ├── server.go          # A2A JSON-RPC server（skill: analyze_critical）
│   ├── gemini.go          # Gemini 根因分析 prompt
│   ├── register.go        # 注冊與心跳邏輯
│   ├── types.go
│   └── go.mod
├── agent3/                # Warning 告警分析 Agent
│   ├── main.go
│   ├── server.go          # A2A JSON-RPC server（skill: analyze_warning）
│   ├── gemini.go          # Gemini 趨勢分析 prompt
│   ├── register.go        # 注冊與心跳邏輯
│   ├── types.go
│   └── go.mod
└── Makefile
```

---

## A2A 核心概念實現

| A2A 概念 | 本系統實現方式 |
|----------|--------------|
| **Agent Card** | `GET /.well-known/agent.json`，描述 Agent 能力與 Skill |
| **Agent Registry** | 獨立服務（port 9000），Agent 啟動時注冊，每 15 秒送心跳 |
| **Task** | `Task` struct，含 ID、SessionID、Messages、Artifacts |
| **Task 生命週期** | submitted → working → completed / failed |
| **Transport** | HTTP POST `/`，JSON-RPC 2.0，Method: `tasks/send` |
| **動態路由** | Agent 1 依 Severity 查 Registry（`analyze_critical` / `analyze_warning`），由 Registry 回傳可用 Agent URL |
| **心跳 TTL** | 45 秒未收到心跳則自動移除，保持 Registry 資料新鮮 |
| **去重機制** | Agent 1 以 fingerprint 為 key 快取已處理告警，告警解除後自動清除快取 |
