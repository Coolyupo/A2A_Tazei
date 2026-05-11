.PHONY: setup setup-registry setup-agent1 setup-agent2 setup-agent3 \
        init-env run-registry run-agent1 run-agent2 run-agent3 \
        test-alert test-alert-image clean

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
	cd agent1 && go get github.com/fsnotify/fsnotify@latest
	cd agent1 && go get github.com/google/uuid@latest
	cd agent1 && go get github.com/joho/godotenv@latest
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

## 啟動 Agent 2（文字分析 Agent）
run-agent2:
	cd agent2 && go run .

## 啟動 Agent 3（圖片分析 Agent）
run-agent3:
	cd agent3 && go run .

## 啟動 Agent 1（監控 Agent，最後啟動）
run-agent1:
	cd agent1 && go run .

## 放入測試 .txt 檔案，觸發文字分析流程
test-alert:
	@mkdir -p agent1/watch
	@echo "系統在凌晨 3:00 偵測到來自 IP 192.168.1.100 的大量失敗登入嘗試，共 500 次，目標帳號為 admin。" \
		> agent1/watch/alert-$$(date +%s).txt
	@echo "已放入測試 .txt 至 agent1/watch/"

## 放入測試圖片，觸發圖片分析流程（1x1 紅色 PNG）
test-alert-image:
	@mkdir -p agent1/watch
	@echo "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwADhQGAWjR9awAAAABJRU5ErkJggg==" \
		| base64 -d > agent1/watch/test-image-$$(date +%s).png
	@echo "已放入測試圖片至 agent1/watch/"

## 清除監控目錄中的測試檔案
clean:
	@rm -f agent1/watch/*.txt agent1/watch/*.png agent1/watch/*.jpg \
		agent1/watch/*.jpeg agent1/watch/*.gif agent1/watch/*.bmp agent1/watch/*.webp
	@echo "已清除 agent1/watch/ 中的測試檔案"
