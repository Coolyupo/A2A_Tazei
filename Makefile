.PHONY: setup setup-registry setup-agent1 setup-agent2 setup-agent3 \
        init-env run-registry run-agent1 run-agent2 run-agent3 \
        test-critical test-warning clean

## 安裝所有依賴（第一次使用請執行）
setup: setup-registry setup-agent2 setup-agent1 setup-agent3

setup-registry:
	@echo "==> 安裝 Registry 依賴..."
	cd registry && go mod tidy

setup-agent2:
	@echo "==> 安裝 Agent 2 依賴..."
	cd agent2 && go get github.com/google/generative-ai-go/genai@latest
	cd agent2 && go get google.golang.org/api@latest
	cd agent2 && go get github.com/google/uuid@latest
	cd agent2 && go get github.com/joho/godotenv@latest
	cd agent2 && go mod tidy

setup-agent1:
	@echo "==> 安裝 Agent 1 依賴..."
	cd agent1 && go get github.com/google/uuid@latest
	cd agent1 && go get github.com/joho/godotenv@latest
	cd agent1 && go get github.com/slack-go/slack@latest
	cd agent1 && go mod tidy

setup-agent3:
	@echo "==> 安裝 Agent 3 依賴..."
	cd agent3 && go get github.com/google/generative-ai-go/genai@latest
	cd agent3 && go get google.golang.org/api@latest
	cd agent3 && go get github.com/joho/godotenv@latest
	cd agent3 && go mod tidy

## 從 .env.example 建立 .env（已存在則略過）
init-env:
	@cp -n agent1/.env.example agent1/.env 2>/dev/null && echo "建立 agent1/.env" || echo "agent1/.env 已存在，略過"
	@cp -n agent2/.env.example agent2/.env 2>/dev/null && echo "建立 agent2/.env" || echo "agent2/.env 已存在，略過"
	@cp -n agent3/.env.example agent3/.env 2>/dev/null && echo "建立 agent3/.env" || echo "agent3/.env 已存在，略過"
	@echo "請編輯 agent2/.env 與 agent3/.env，填入您的 GEMINI_API_KEY"

## 啟動 Registry（最先啟動）
run-registry:
	cd registry && go run .

## 啟動 Agent 2（Critical 告警分析 Agent）
run-agent2:
	cd agent2 && go run .

## 啟動 Agent 3（Warning 告警分析 Agent）
run-agent3:
	cd agent3 && go run .

## 啟動 Agent 1（Alertmanager 監控 Agent，最後啟動）
run-agent1:
	cd agent1 && go run .

## 送出 Critical 測試告警到 Alertmanager
test-critical:
	@curl -s -X POST http://localhost:9093/api/v2/alerts \
		-H "Content-Type: application/json" \
		-d '[{"labels":{"alertname":"HighCPUUsage","severity":"critical","instance":"prod-server-01","job":"node-exporter","env":"production"},"annotations":{"summary":"CPU 使用率超過 95%","description":"prod-server-01 的 CPU 使用率已持續 5 分鐘超過 95%，可能導致服務降級或中斷"}}]'
	@echo "已送出 Critical 測試告警至 Alertmanager (localhost:9093)"

## 送出 Warning 測試告警到 Alertmanager
test-warning:
	@curl -s -X POST http://localhost:9093/api/v2/alerts \
		-H "Content-Type: application/json" \
		-d '[{"labels":{"alertname":"HighMemoryUsage","severity":"warning","instance":"staging-server-02","job":"node-exporter","env":"staging"},"annotations":{"summary":"記憶體使用率超過 80%","description":"staging-server-02 的記憶體使用率已達 82%，若持續上升可能影響服務穩定性"}}]'
	@echo "已送出 Warning 測試告警至 Alertmanager (localhost:9093)"

## 同時送出 Critical 與 Warning 測試告警
test-all: test-critical test-warning
