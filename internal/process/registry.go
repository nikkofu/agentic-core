package process

import (
	"agentic-core/internal/memory"
	"fmt"
	"sync"
)

type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]memory.AgentProfile
}

func NewAgentRegistry() *AgentRegistry {
	r := &AgentRegistry{
		agents: make(map[string]memory.AgentProfile),
	}
	r.registerDefaults()
	return r
}

func (r *AgentRegistry) registerDefaults() {
	r.Register(memory.AgentProfile{
		Name:        "orchestrator",
		Description: "The team lead that manages workflows and delegates tasks.",
		RolePrompt:  "You are the Orchestrator. Your job is to break down complex tasks and delegate them to specialized agents using @agent_name.",
	})
	r.Register(memory.AgentProfile{
		Name:        "researcher",
		Description: "Specializes in searching information and gathering data.",
		RolePrompt:  "You are a Researcher. Your goal is to find accurate and up-to-date information on the given topic.",
	})
	r.Register(memory.AgentProfile{
		Name:        "coder",
		Description: "Expert in writing and debugging Go, Python, and other languages.",
		RolePrompt:  "You are an Expert Coder. You write clean, efficient, and well-documented code.",
	})
}

func (r *AgentRegistry) Register(profile memory.AgentProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[profile.Name] = profile
}

func (r *AgentRegistry) GetProfile(name string) (memory.AgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	profile, ok := r.agents[name]
	if !ok {
		return memory.AgentProfile{}, fmt.Errorf("agent not found: %s", name)
	}
	return profile, nil
}

func (r *AgentRegistry) ListAgents() []memory.AgentProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []memory.AgentProfile
	for _, p := range r.agents {
		list = append(list, p)
	}
	return list
}
