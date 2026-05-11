package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	reg := NewRegistry()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /agents/register", func(w http.ResponseWriter, r *http.Request) {
		var card AgentCard
		if err := json.NewDecoder(r.Body).Decode(&card); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reg.Register(card)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
	})

	mux.HandleFunc("POST /agents/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !reg.Heartbeat(body.URL) {
			http.Error(w, "agent not found, please re-register", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /agents/deregister", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reg.Deregister(body.URL)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /agents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if skill := r.URL.Query().Get("skill"); skill != "" {
			json.NewEncoder(w).Encode(reg.FindBySkill(skill))
		} else {
			json.NewEncoder(w).Encode(reg.List())
		}
	})

	log.Println("[Registry] Agent Registry 啟動於 :9000")
	log.Println("[Registry] GET  http://localhost:9000/agents")
	log.Println("[Registry] POST http://localhost:9000/agents/register")
	if err := http.ListenAndServe(":9000", mux); err != nil {
		log.Fatalf("[Registry] 錯誤：%v", err)
	}
}
