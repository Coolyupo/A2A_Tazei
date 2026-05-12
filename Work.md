# Work Log

## v5.0 — Agent 2 MCP Client + Slack Bot Human-in-the-Loop 核准流程

### 架構變更

**Agent 2 — MCP Client + 兩階段任務**：

- 新增 `mcp_client.go`：MCP SSE Client，支援 `tools/list` / `tools/call`。連線時自動執行 `initialize` + `notifications/initialized` 握手
- `gemini.go` 新增 `selectToolWithGemini()`：用 ResponseMIMEType + ResponseSchema 強制 JSON，從 MCP 工具清單中選擇最適工具，無合適工具時選 `builtin`
- `gemini.go` 新增 `executeBuiltinAnalysis()`：原 `analyzeWithGemini()` 重命名，作為內建 fallback
- `server.go` 支援兩種 RPC Method：
  - `tasks/send`（Phase 1）：分析 + 選工具 → `state: awaiting_approval` + 工具資訊寫入 `Metadata`
  - `tasks/approve`（Phase 2）：從 pending store 取出任務 → 執行 MCP tool 或 builtin → `state: completed`
- `server.go` 加入 pending task store（`map[string]*PendingTask` + `sync.RWMutex`）及 15 分鐘 TTL 清理 goroutine
- `main.go` 啟動時嘗試連線 `MCP_SERVER_URL`，失敗則以 builtin 模式繼續
- `types.go` 新增 `MCPTool`、`ToolChoice`、`PendingTask`、`Task.Metadata`
- `.env.example` 新增 `MCP_SERVER_URL`

**Agent 1 — Slack Bot Socket Mode + 兩階段 Critical 流程**：

- 新增 `slack_bot.go`：
  - `SlackBot`：封裝 `slack.Client` + `socketmode.Client`，Background 啟動 Socket Mode
  - `ApprovalManager`：管理 `map[string]*PendingApproval`（thread-safe），每個 PendingApproval 帶 `ResultCh chan *ApprovalResult`
  - `PostApprovalRequest()`：發送 Block Kit 訊息，帶 `[✅ Approve]` 按鈕，`action_id=approve_task`，`value=taskID`
  - Socket Mode 收到 `InteractionTypeBlockActions` 後 Ack → 查找 pending → 呼叫 `client.ApproveTask()` → 送結果到 `ResultCh`
  - 逾時後更新 Slack 訊息為「已過期」
- `a2a_client.go` 新增 `ApproveTask(taskID string)`：發送 `tasks/approve` JSON-RPC 到 Agent 2
- `alertmanager.go` 重構：
  - `routeCritical()`：Phase 1 → Slack 核准（10 分鐘 `select`） → Phase 2 → 結果送 Slack
  - `routeWarning()`：維持原 Agent 3 Agentic 路由
  - Slack Bot 未設定時自動核准（`autoApproveAndNotify`）
- `main.go` 讀取 `SLACK_BOT_TOKEN`、`SLACK_APP_TOKEN`、`SLACK_CHANNEL_ID`，初始化 Bot
- `.env.example` 新增三個 Slack Bot 環境變數

**新增依賴**：
- `agent1/go.mod`：`github.com/slack-go/slack v0.23.1`

### 完整流程

```
Critical 告警 → Agent 1 → Agent 2 tasks/send
                              ├─ MCP tools/list（有設定則呼叫）
                              ├─ Gemini 選工具（or builtin）
                              └─ 回傳 awaiting_approval
           Agent 1 ─── Slack Bot PostMessage（Approve 按鈕）
           Agent 1 ─── 等待 10 分鐘
           User ──────── 點 [✅ Approve]
           SlackBot ──── Agent 2 tasks/approve
                              └─ 執行 MCP tool / builtin
                              └─ 回傳 completed
           Agent 1 ─── Slack Incoming Webhook（結果）
```

### 環境準備

```bash
# Slack App 需要設定：
# 1. 啟用 Socket Mode，建立 App-Level Token（scope: connections:write）
# 2. Bot Token Scopes：chat:write
# 3. 啟用 Interactivity

echo "SLACK_BOT_TOKEN=xoxb-..." >> agent1/.env
echo "SLACK_APP_TOKEN=xapp-..." >> agent1/.env
echo "SLACK_CHANNEL_ID=C0123456789" >> agent1/.env

# MCP Server（Agent 2，選填）
echo "MCP_SERVER_URL=http://your-mcp-server:3000" >> agent2/.env
```

### 新增/修改檔案

| 檔案 | 說明 |
|------|------|
| `agent2/mcp_client.go` | MCP SSE Client（新增） |
| `agent2/gemini.go` | selectToolWithGemini + executeBuiltinAnalysis（改寫） |
| `agent2/server.go` | tasks/send Phase 1 + tasks/approve Phase 2（改寫） |
| `agent2/types.go` | MCPTool, ToolChoice, PendingTask, Task.Metadata（新增） |
| `agent2/main.go` | 初始化 MCP Client（更新） |
| `agent2/.env.example` | 新增 MCP_SERVER_URL |
| `agent1/slack_bot.go` | SlackBot + ApprovalManager（新增） |
| `agent1/a2a_client.go` | 新增 ApproveTask()（更新） |
| `agent1/alertmanager.go` | routeCritical 兩階段 + routeWarning（改寫） |
| `agent1/main.go` | 初始化 Slack Bot（更新） |
| `agent1/.env.example` | 新增 Bot/App Token、Channel ID |

---

## v4.0 — Agent 3 加入 Agentic 決策 + Agent 1 加入 Slack 通知

### 架構變更

**Agent 3 — Agentic Warning 分析**：

- 移除單純分析回傳的模式，改為 **Agentic Loop**：Gemini 不只分析，同時做出自主決策（`silence` / `escalate`）
- 新增 `analyzeAndDecide()` 函式：一次呼叫 Gemini 同時產出完整分析報告與決策 JSON `{"decision":"...", "reason":"...", "analysis":"..."}`
- 新增 `alertmanager.go`：當 Gemini 判定為不重要時，自主呼叫 Alertmanager `POST /api/v2/silences`，建立 48 小時 Silence（以告警原始 labels 作為 matchers）
- `server.go` 在 Task 完成後附帶 `Metadata`：`action`（silence/escalate）、`reason`（AI 決策理由）、`silence_id`（靜默 ID）
- 更新 `.env.example`：新增 `ALERTMANAGER_URL`

**Agent 1 — Slack 通知整合**：

- 新增 `slack.go`：Slack Incoming Webhook 發送，支援 Attachment 格式（color-coded：紅色 critical、橘色 warning 升級）
- `alertmanager.go` 更新 `routeAlert()`：
  - Critical 告警 → Agent 2 分析完成後一律送 Slack
  - Warning 告警 → Agent 3 回傳 `action=escalate` 時送 Slack，`action=silence` 時只記錄 log
- `main.go` 讀取 `SLACK_WEBHOOK_URL` 環境變數並傳入 Poller
- 更新 `.env.example`：新增 `SLACK_WEBHOOK_URL`

**跨 Agent Task Metadata**：

- `agent1/types.go` 與 `agent3/types.go` 的 `Task` struct 新增 `Metadata map[string]string`
- Agent 1 送 Task 時附帶 `fingerprint` 與 `labels`（JSON 字串），供 Agent 3 建立 Silence 時使用
- Agent 3 回傳時附帶 `action`、`reason`、`silence_id`，供 Agent 1 判斷是否發送 Slack

### 決策流程

```
Warning 告警進來
    │
    ▼
Agent 3 Gemini 分析
    │
    ├── decision=silence → POST /api/v2/silences（48h）→ 回傳 action=silence
    │                                                      Agent 1 記 log，不通知
    │
    └── decision=escalate → 回傳 action=escalate
                             Agent 1 送 Slack 通知

Critical 告警進來
    │
    ▼
Agent 2 Gemini 根因分析
    │
    └── 一律 → Agent 1 送 Slack 通知
```

### 環境準備

```bash
# Slack Webhook 設定（選填，不設定則只輸出 log）
echo "SLACK_WEBHOOK_URL=https://hooks.slack.com/services/..." >> agent1/.env

# Alertmanager URL（agent3 用於建立 silence，預設已指向 localhost:9093）
echo "ALERTMANAGER_URL=http://localhost:9093" >> agent3/.env
```

### 新增檔案

| 檔案 | 說明 |
|------|------|
| `agent3/alertmanager.go` | Alertmanager Silence API 呼叫邏輯 |
| `agent1/slack.go` | Slack Incoming Webhook 發送 |

### 修改檔案

| 檔案 | 修改重點 |
|------|---------|
| `agent3/gemini.go` | 改為 `analyzeAndDecide()`，返回結構化 JSON 決策 |
| `agent3/server.go` | Agentic 決策分支：呼叫 silence API 或設定 escalate |
| `agent3/types.go` | 新增 `AnalysisResult`、`Task.Metadata` |
| `agent3/.env.example` | 新增 `ALERTMANAGER_URL` |
| `agent1/alertmanager.go` | `routeAlert` 依 action 決定是否送 Slack |
| `agent1/main.go` | 讀取並傳遞 `SLACK_WEBHOOK_URL` |
| `agent1/a2a_client.go` | Task 附帶 `Metadata`（fingerprint + labels） |
| `agent1/types.go` | 新增 `Task.Metadata` |
| `agent1/.env.example` | 新增 `SLACK_WEBHOOK_URL` |

---

## v3.0 — Agent 1 改為監控 Alertmanager，依 Severity 分流告警

### 架構變更

Agent 1 的監控來源從本地目錄（`./watch`）改為 Alertmanager HTTP API，告警分流邏輯依 Severity 路由至不同 Agent：

- **修改 Agent 1**：移除 `fsnotify` 目錄監控，改為每 30 秒輪詢 `http://localhost:9093/api/v2/alerts`；以 fingerprint 做去重避免重複觸發；依 `severity=critical` / `warning` 向 Registry 查詢對應 Agent 並送出 Task；環境變數從 `WATCH_DIR` 改為 `ALERTMANAGER_URL` 與 `POLL_INTERVAL`
- **修改 Agent 2**：Skill ID 從 `analyze_txt` 改為 `analyze_critical`；Agent 名稱改為 `CriticalAlertAnalyzerAgent`；Gemini prompt 改為 Critical 告警根因分析（影響評估 + 緊急處置建議）
- **修改 Agent 3**：Skill ID 從 `analyze_image` 改為 `analyze_warning`；Agent 名稱改為 `WarningAlertAnalyzerAgent`；server.go 從圖片解析改為文字解析；Gemini prompt 改為 Warning 告警趨勢分析（潛在風險 + 預防建議）
- **修改 Makefile**：移除 `fsnotify` 安裝步驟；移除舊的 `test-alert` / `test-alert-image` 目標；新增 `test-critical`、`test-warning`、`test-all` 三個往 Alertmanager 注入測試告警的目標

### 路由規則

```
severity=critical → skill: analyze_critical → Agent 2（根因分析）
severity=warning  → skill: analyze_warning  → Agent 3（趨勢分析）
其他 severity     → 忽略
```

### 告警去重機制

Agent 1 以 Alertmanager 回傳的 `fingerprint` 欄位為 key，快取已處理的告警。告警狀態變為非 active 後，從快取中移除，允許同一告警再次觸發時重新分派。

### 啟動順序

**必須依序啟動**，Registry 要最先起來，Agent 1 最後：

```bash
# 終端機 1
make run-registry

# 終端機 2
make run-agent2

# 終端機 3
make run-agent3

# 終端機 4
make run-agent1
```

### 環境準備

第一次使用：

```bash
make setup       # 安裝所有依賴（agent1 不再需要 fsnotify）
make init-env    # 建立 .env 檔案

# 填入 Gemini API Key（agent2 和 agent3 各需填一次）
echo "GEMINI_API_KEY=你的key" >> agent2/.env
echo "GEMINI_API_KEY=你的key" >> agent3/.env
```

### 測試方式

```bash
# 送出 Critical 測試告警（觸發 Agent 2 分析）
make test-critical

# 送出 Warning 測試告警（觸發 Agent 3 分析）
make test-warning

# 同時送出兩筆告警
make test-all
```

> Agent 1 預設每 30 秒輪詢一次，可透過 `POLL_INTERVAL` 環境變數調整（如 `POLL_INTERVAL=10s`）。

---

## v2.0 — 加入 Agent Registry 與 Agent 3（圖片分析）

### 架構變更

原本的 P2P 架構（Agent1 硬寫 Agent2 URL）改為 Registry-based 架構：

- **新增 `registry/`**：中央服務發現服務（port 9000），維護所有 Agent 的 AgentCard，提供 TTL 心跳機制
- **新增 `agent3/`**：圖片分析 Agent（port 8081），使用 Gemini Vision 分析 jpg/png/gif/bmp/webp
- **修改 Agent2**：改名為 TextAnalyzerAgent，Skill ID 從 `analyze_file` 改為 `analyze_txt`，啟動時主動向 Registry 注冊並每 15 秒送心跳
- **修改 Agent1**：移除硬寫的 `AGENT2_URL`，改為查詢 Registry；監控範圍從僅 `.txt` 擴展為文字 + 圖片；圖片內容 base64 編碼後傳給 Agent3

### 啟動順序

**必須依序啟動**，Registry 要最先起來，Agent1 最後：

```bash
# 終端機 1
make run-registry

# 終端機 2
make run-agent2

# 終端機 3
make run-agent3

# 終端機 4
make run-agent1
```

### 環境準備

第一次使用：

```bash
make setup       # 安裝所有依賴（含 registry 和 agent3）
make init-env    # 建立 .env 檔案

# 填入 Gemini API Key（agent2 和 agent3 各需填一次）
echo "GEMINI_API_KEY=你的key" >> agent2/.env
echo "GEMINI_API_KEY=你的key" >> agent3/.env
```

### 測試方式

```bash
# 文字分析（觸發 Agent2）
make test-alert

# 圖片分析（觸發 Agent3，放入 1x1 PNG）
make test-alert-image

# 手動放入任意支援格式的檔案
cp photo.png agent1/watch/
echo "任意文字" > agent1/watch/note.txt

# 清除測試檔案
make clean
```

### 查看 Registry 狀態

```bash
# 列出所有已注冊 Agent
curl http://localhost:9000/agents | jq .

# 查詢特定 Skill 的 Agent
curl "http://localhost:9000/agents?skill=analyze_txt" | jq .
curl "http://localhost:9000/agents?skill=analyze_image" | jq .
```

---

## v1.0 — 初始版本（P2P 架構）

兩個 Agent 直接連線：Agent1 監控 `.txt` 檔案，偵測到後送給 Agent2（Gemini 分析）。
Agent1 透過 `.env` 的 `AGENT2_URL` 硬寫 Agent2 位址。
