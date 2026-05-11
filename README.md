# A2A Multi-Agent System

以 [A2A（Agent-to-Agent）協議](https://google.github.io/A2A/) 為基礎的多 Agent 系統，透過中央 Registry 做動態服務發現與路由。

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
          │ analyze_txt     │              │ analyze_image   │
          │                 │              │                 │
          │ 處理 .txt 檔案  │              │ 處理圖片檔案    │
          │ Gemini Flash    │              │ Gemini Vision   │
          └────────▲────────┘              └────────▲────────┘
                   │                                │
                   │     A2A JSON-RPC tasks/send    │
                   │                                │
          ┌────────┴────────────────────────────────┴────────┐
          │                   Agent 1                        │
          │              監控 ./watch 目錄                    │
          │                                                   │
          │  .txt  → 查 Registry(analyze_txt)  → Agent 2     │
          │  圖片  → 查 Registry(analyze_image) → Agent 3    │
          └──────────────────────────────────────────────────┘
```

### 元件說明

| 元件 | 埠號 | 職責 |
|------|------|------|
| **Registry** | 9000 | Agent 注冊中心，維護可用 Agent 清單，45 秒心跳 TTL |
| **Agent 1** | — | 監控 watch 目錄，依檔案類型向 Registry 查詢並路由 Task |
| **Agent 2** | 8080 | 接收 `.txt` 文字檔，用 Gemini 分析異常 |
| **Agent 3** | 8081 | 接收圖片檔（jpg/png/gif/bmp/webp），用 Gemini Vision 分析 |

### 支援的檔案格式

| 副檔名 | 路由至 | Skill ID |
|--------|--------|----------|
| `.txt` | Agent 2 | `analyze_txt` |
| `.jpg` `.jpeg` `.png` `.gif` `.bmp` `.webp` | Agent 3 | `analyze_image` |

---

## 快速開始

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

# 終端機 2：Agent 2（文字分析）
make run-agent2

# 終端機 3：Agent 3（圖片分析）
make run-agent3

# 終端機 4：Agent 1（監控，最後啟動）
make run-agent1
```

### 4. 測試

```bash
# 測試文字分析（觸發 Agent 2）
make test-alert

# 測試圖片分析（觸發 Agent 3）
make test-alert-image

# 手動放入任意檔案
echo "你想分析的內容" > agent1/watch/test.txt
cp /path/to/photo.png agent1/watch/
```

### 5. 清除測試檔案

```bash
make clean
```

---

## Registry API

| Method | Path | 說明 |
|--------|------|------|
| `GET` | `/agents` | 列出所有已注冊 Agent |
| `GET` | `/agents?skill=analyze_txt` | 依 skill 篩選 Agent |
| `POST` | `/agents/register` | 注冊 Agent（body: AgentCard JSON）|
| `POST` | `/agents/heartbeat` | 送出心跳（body: `{"url":"..."}`）|
| `POST` | `/agents/deregister` | 主動下線（body: `{"url":"..."}`）|

查看已注冊的 Agent：

```bash
curl http://localhost:9000/agents | jq .
curl "http://localhost:9000/agents?skill=analyze_txt" | jq .
```

---

## 環境變數

### Agent 1

| 環境變數 | 預設值 | 說明 |
|----------|--------|------|
| `WATCH_DIR` | `./watch` | 監控目錄 |
| `REGISTRY_URL` | `http://localhost:9000` | Registry 位址 |

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
A2A/
├── registry/          # Agent Registry 服務
│   ├── main.go        # HTTP Server，提供注冊/心跳/查詢 API
│   ├── registry.go    # 注冊、心跳、TTL 過期邏輯
│   ├── types.go
│   └── go.mod
├── agent1/            # 監控 Agent
│   ├── main.go
│   ├── watcher.go     # 目錄監控，依副檔名路由
│   ├── a2a_client.go  # RegistryClient + A2AClient
│   ├── types.go
│   └── go.mod
├── agent2/            # 文字分析 Agent（.txt）
│   ├── main.go
│   ├── server.go      # A2A JSON-RPC server
│   ├── gemini.go      # Gemini 文字分析
│   ├── register.go    # 注冊與心跳邏輯
│   ├── types.go
│   └── go.mod
├── agent3/            # 圖片分析 Agent（jpg/png/gif/bmp/webp）
│   ├── main.go
│   ├── server.go      # A2A JSON-RPC server
│   ├── gemini.go      # Gemini Vision 圖片分析
│   ├── register.go    # 注冊與心跳邏輯
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
| **動態路由** | Agent 1 依副檔名查 Registry，由 Registry 回傳可用 Agent URL |
| **心跳 TTL** | 45 秒未收到心跳則自動移除，保持 Registry 資料新鮮 |
