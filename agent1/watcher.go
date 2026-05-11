package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

var supportedExtensions = map[string]string{
	".txt":  "analyze_txt",
	".jpg":  "analyze_image",
	".jpeg": "analyze_image",
	".png":  "analyze_image",
	".gif":  "analyze_image",
	".bmp":  "analyze_image",
	".webp": "analyze_image",
}

var mimeTypes = map[string]string{
	".txt":  "text/plain",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
}

func StartWatcher(watchDir string, registry *RegistryClient) error {
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		return fmt.Errorf("建立監控目錄失敗：%w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("初始化 fsnotify 失敗：%w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(watchDir); err != nil {
		return fmt.Errorf("加入監控目錄失敗：%w", err)
	}

	log.Printf("[Agent1] 開始監控目錄：%s", watchDir)
	log.Println("[Agent1] 支援格式：.txt / .jpg / .jpeg / .png / .gif / .bmp / .webp")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&fsnotify.Create == 0 {
				continue
			}
			ext := strings.ToLower(filepath.Ext(event.Name))
			skill, ok := supportedExtensions[ext]
			if !ok {
				continue
			}
			log.Printf("[Agent1] 偵測到檔案：%s (skill: %s)", event.Name, skill)
			time.Sleep(100 * time.Millisecond) // 確保檔案寫入完成
			go processFile(event.Name, ext, skill, registry)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("[Agent1] 監控錯誤：%v", err)
		}
	}
}

func processFile(path, ext, skill string, registry *RegistryClient) {
	agentURL, err := registry.FindAgent(skill)
	if err != nil {
		log.Printf("[Agent1] 找不到可用的 Agent（skill: %s）：%v", skill, err)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[Agent1] 讀取檔案失敗 (%s)：%v", path, err)
		return
	}

	var content string
	mimeType := mimeTypes[ext]

	if skill == "analyze_txt" {
		content = string(data)
	} else {
		content = base64.StdEncoding.EncodeToString(data)
	}

	client := NewA2AClient(agentURL)
	result, err := client.SendTask(filepath.Base(path), content, mimeType)
	if err != nil {
		log.Printf("[Agent1] Task 執行失敗：%v", err)
		return
	}

	printResult(result)
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
