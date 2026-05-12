# A2A Multi-Agent System

以 [A2A（Agent-to-Agent）協議](https://google.github.io/A2A/) 為基礎的多 Agent 系統，透過中央 Registry 做動態服務發現與路由。

---

## 系統架構

```
                    ┌─────────────────────────────┐
                    │         Registry             │
                    │      localhost:9000          │
                    └──────────┬──────────┬────────┘
                               │          │
                    ┌──────────┘          └──────────┐
                    ▼                                 ▼
     ┌──────────────────────────┐      ┌─────────────────────────────┐
     │         Agent 2          │      │         Agent 3             │
     │      localhost:8080      │      │      localhost:8081         │
     │                          │      │                             │
     │  skill: analyze_critical │      │  skill: analyze_warning     │
     │                          │      │                             │
     │  ① tasks/send            │      │  Agentic 決策：             │
     │    Gemini 分析告警        │      │  ├─ silence → Alertmanager  │
     │    MCP tools/list        │      │  │   POST /api/v2/silences  │
     │    選擇工具               │      │  └─ escalate → 回傳 Agent1  │
     │    → awaiting_approval   │      │                             │
     │                          │      │  Gemini Flash               │
     │  ② tasks/approve         │      └──────────────▲─────────────┘
     │    執行 MCP tool          │                     │ severity=warning
     │    或 builtin 分析        │                     │
     │    → completed           │      ┌──────────────┴─────────────────────────────────┐
     │                          │      │                   Agent 1                       │
     │  MCP Client (SSE)        │      │         輪詢 Alertmanager（每 30 秒）            │
     └───────────▲──────────────┘      │                                                 │
                 │ ① tasks/send        │  severity=critical → Agent 2 兩階段流程：       │
                 │ ② tasks/approve     │    Phase 1: 工具選擇（awaiting_approval）       │
                 │                     │    → Slack 發核准按鈕（Socket Mode Bot）        │
                 │                     │    → User 在 Slack 點 Approve（10 分鐘內）      │
                 │                     │    Phase 2: tasks/approve → 工具執行            │
                 │                     │    → 結果送 Slack Incoming Webhook              │
                 │                     │                                                 │
                 └─────────────────────┤  severity=warning → Agent 3 Agentic 決策       │
                                       │    → silence：記錄 log                         │
                                       │    → escalate：送 Slack Incoming Webhook       │
                                       └──────────────┬──────────────────────────────────┘
                                                      │
                              ┌───────────────────────┴────────────────────┐
                              │               Slack                        │
                              │                                            │
                              │  Bot（Socket Mode）                        │
                              │  ┌─────────────────────────────────┐      │
                              │  │ ⚠️ Critical 告警工具核准請求      │      │
                              │  │ 告警：HighCPUUsage               │      │
                              │  │ 工具：restart_service (MCP)      │      │
                              │  │ [✅ Approve]                     │      │
                              │  └─────────────────────────────────┘      │
                              │                                            │
                              │  Incoming Webhook（結果通知）               │
                              │  ┌─────────────────────────────────┐      │
                              │  │ 🚨 [Critical] HighCPUUsage       │      │
                              │  │ 執行結果：...                    │      │
                              │  └─────────────────────────────────┘      │
                              └────────────────────────────────────────────┘
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
| **Registry** | 9000 | Agent 注冊中心，45 秒心跳 TTL |
| **Agent 1** | — | 輪詢 Alertmanager；Critical 兩階段流程 + Slack Bot 核准；Warning Agentic 路由 |
| **Agent 2** | 8080 | Critical 告警：MCP Client 選工具 → 等核准 → 執行；內建 Gemini 分析為 fallback |
| **Agent 3** | 8081 | Warning 告警：Gemini 自主決策（silence 48h / escalate） |
| **Alertmanager** | 9093 | 告警來源 |
| **MCP Server** | 自訂 | 提供可執行工具（Agent 2 透過 SSE 連線） |
| **Slack Bot** | Socket Mode | 接收 Approve 按鈕互動，不揭露外部接口 |
| **Slack Webhook** | — | 接收分析結果通知 |

---

## Critical 告警兩階段流程

```
Agent 1 收到 Critical 告警
          │
          ▼
  tasks/send → Agent 2
          │
          ├─ MCP ListTools（若設定 MCP_SERVER_URL）
          │
          ├─ Gemini 分析告警 + 選擇工具
          │   ├─ 有合適 MCP 工具 → 選 MCP 工具
          │   └─ 無合適工具     → 選 builtin（Gemini 分析）
          │
          └─ 回傳 state=awaiting_approval + 工具選擇
                    │
                    ▼
          Agent 1 發 Slack 核准訊息（Bot + 按鈕）
                    │
           ┌────────┴────────┐
           │  等待最多 10 分鐘  │
           └────────┬────────┘
     User 點 Approve │              逾時
           │        │               │
           ▼        │               ▼
   tasks/approve    │       更新 Slack 訊息：已過期
   → Agent 2        │
   執行工具           │
   回傳結果           │
           │
           ▼
   Agent 1 送 Slack Incoming Webhook（結果）
```

### Agent 3 Warning Agentic 流程

```
Agent 3 收到 Warning 告警 → Gemini 分析並決策
  ├─ silence → POST /api/v2/silences（48h）→ 回傳 action=silence
  └─ escalate → 回傳 action=escalate → Agent 1 送 Slack
```

---

## Slack App 設定

### 需要的 Scopes

**Bot Token Scopes（`xoxb-...`）：**
- `chat:write` — 發送訊息
- `chat:write.public` — 發送到未加入的頻道（選填）

**App-Level Token Scopes（`xapp-...`）：**
- `connections:write` — Socket Mode 連線

### 設定步驟

1. 前往 [api.slack.com/apps](https://api.slack.com/apps) 建立 App
2. **Socket Mode**：啟用 Socket Mode，建立 App-Level Token（scope: `connections:write`）
3. **Interactivity**：啟用 Interactivity（Socket Mode 下不需填 URL）
4. **OAuth & Permissions**：加入 `chat:write` scope，安裝 App 到 Workspace
5. 將 Bot 加入目標頻道

---

## 快速開始

### 0. 啟動 Alertmanager

```bash
docker run -d --name alertmanager -p 9093:9093 prom/alertmanager
```

### 1. 安裝依賴

```bash
make setup
```

### 2. 建立 .env 設定檔

```bash
make init-env
```

填入各 Agent 設定：

```bash
# Agent 2：Gemini API Key（必填）+ MCP Server URL（選填）
echo "GEMINI_API_KEY=your_key" >> agent2/.env
echo "MCP_SERVER_URL=http://your-mcp-server:3000" >> agent2/.env  # 選填

# Agent 3：Gemini API Key
echo "GEMINI_API_KEY=your_key" >> agent3/.env

# Agent 1：Slack 設定
echo "SLACK_WEBHOOK_URL=https://hooks.slack.com/services/..." >> agent1/.env
echo "SLACK_BOT_TOKEN=xoxb-..." >> agent1/.env      # Critical 核准按鈕用
echo "SLACK_APP_TOKEN=xapp-..." >> agent1/.env       # Socket Mode 用
echo "SLACK_CHANNEL_ID=C0123456789" >> agent1/.env  # 核准訊息頻道
```

> `SLACK_BOT_TOKEN` / `SLACK_APP_TOKEN` 未設定時，Critical 告警跳過核准步驟自動執行。

### 3. 啟動系統

```bash
make run-registry  # 終端機 1（最先啟動）
make run-agent2    # 終端機 2
make run-agent3    # 終端機 3
make run-agent1    # 終端機 4（最後啟動）
```

### 4. 測試

```bash
make test-critical   # 觸發 Agent 2 兩階段流程 → Slack 核准
make test-warning    # 觸發 Agent 3 Agentic 決策
make test-all        # 同時送出兩筆
```

---

## 環境變數

### Agent 1

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `ALERTMANAGER_URL` | `http://localhost:9093` | Alertmanager 位址 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `POLL_INTERVAL` | `30s` | 輪詢間隔 |
| `SLACK_WEBHOOK_URL` | — | Incoming Webhook URL（結果通知） |
| `SLACK_BOT_TOKEN` | — | Bot Token（`xoxb-`，核准按鈕用） |
| `SLACK_APP_TOKEN` | — | App-Level Token（`xapp-`，Socket Mode 用） |
| `SLACK_CHANNEL_ID` | — | 核准訊息發送的頻道 ID |

### Agent 2

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `GEMINI_API_KEY` | （必填）| Gemini API 金鑰 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `SELF_URL` | `http://localhost:8080` | 自身對外 URL |
| `PORT` | `8080` | 監聽埠號 |
| `MCP_SERVER_URL` | — | MCP Server URL（不填則 builtin 模式） |

### Agent 3

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `GEMINI_API_KEY` | （必填）| Gemini API 金鑰 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |
| `SELF_URL` | `http://localhost:8081` | 自身對外 URL |
| `PORT` | `8081` | 監聽埠號 |
| `ALERTMANAGER_URL` | `http://localhost:9093` | 用於建立 Silence |

---

## 目錄結構

```
A2A_TaZei/
├── registry/
│   ├── main.go / registry.go / types.go / go.mod
├── agent1/                      # Orchestrator Agent
│   ├── main.go                  # 啟動 Poller + Slack Bot
│   ├── alertmanager.go          # 輪詢 + 兩階段 Critical 流程 + Warning 路由
│   ├── slack_bot.go             # Socket Mode Bot + ApprovalManager
│   ├── slack.go                 # Incoming Webhook 結果通知
│   ├── a2a_client.go            # SendAlertTask + ApproveTask
│   ├── types.go                 # A2A 型別（Task.Metadata）
│   └── go.mod
├── agent2/                      # Critical 告警分析 Agent（MCP Client）
│   ├── main.go                  # 啟動 + 初始化 MCP Client
│   ├── server.go                # tasks/send（Phase 1）+ tasks/approve（Phase 2）
│   ├── mcp_client.go            # MCP SSE Client（tools/list + tools/call）
│   ├── gemini.go                # selectToolWithGemini + executeBuiltinAnalysis
│   ├── register.go / types.go / go.mod
├── agent3/                      # Warning 告警 Agentic 決策 Agent
│   ├── server.go / gemini.go / alertmanager.go / register.go / types.go / go.mod
└── Makefile
```

---

## A2A 核心概念實現

| A2A 概念 | 本系統實現方式 |
|----------|--------------|
| **Agent Card** | `GET /.well-known/agent.json` |
| **Agent Registry** | 獨立服務（port 9000），15 秒心跳 |
| **Task** | `Task` struct，含 Metadata（工具選擇、決策） |
| **Task 生命週期** | submitted → working → awaiting_approval → completed / failed |
| **Transport** | HTTP POST `/`，JSON-RPC 2.0 |
| **動態路由** | Agent 1 依 Severity 查 Registry |
| **MCP Client** | Agent 2 透過 SSE 協議連線 MCP Server，發現並執行工具 |
| **Human-in-the-Loop** | Slack Bot（Socket Mode）核准按鈕，10 分鐘逾時 |
| **Agentic 決策** | Agent 3 自主 Silence 或升級；Agent 2 自主選工具 |
| **外部通知** | Slack Incoming Webhook（結果）+ Socket Mode（互動） |
