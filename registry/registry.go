package main

import (
	"log"
	"sync"
	"time"
)

const heartbeatTTL = 45 * time.Second

type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentEntry // key: agent URL
}

func NewRegistry() *Registry {
	r := &Registry{agents: make(map[string]*AgentEntry)}
	go r.expireLoop()
	return r
}

func (r *Registry) Register(card AgentCard) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if entry, exists := r.agents[card.URL]; exists {
		entry.Card = card
		entry.LastSeen = now
		return
	}
	r.agents[card.URL] = &AgentEntry{Card: card, RegisteredAt: now, LastSeen: now}
	log.Printf("[Registry] 已注冊：%s (%s)", card.Name, card.URL)
}

func (r *Registry) Heartbeat(url string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.agents[url]
	if !exists {
		return false
	}
	entry.LastSeen = time.Now()
	return true
}

func (r *Registry) Deregister(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, exists := r.agents[url]; exists {
		log.Printf("[Registry] 已下線：%s (%s)", entry.Card.Name, url)
	}
	delete(r.agents, url)
}

func (r *Registry) FindBySkill(skillID string) []AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AgentCard
	for _, entry := range r.agents {
		for _, skill := range entry.Card.Skills {
			if skill.ID == skillID {
				result = append(result, entry.Card)
				break
			}
		}
	}
	return result
}

func (r *Registry) List() []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentEntry, 0, len(r.agents))
	for _, entry := range r.agents {
		result = append(result, *entry)
	}
	return result
}

func (r *Registry) expireLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		for url, entry := range r.agents {
			if time.Since(entry.LastSeen) > heartbeatTTL {
				log.Printf("[Registry] 心跳超時，移除：%s (%s)", entry.Card.Name, url)
				delete(r.agents, url)
			}
		}
		r.mu.Unlock()
	}
}
