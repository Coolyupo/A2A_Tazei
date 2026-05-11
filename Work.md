# Work Log

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
