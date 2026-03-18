package skill

import (
	"context"
	"encoding/json"
)

// Skill 定义了 Agent 可以执行的一个原子动作
type Skill interface {
	Name() string
	Description() string
	Schema() string // 返回参数的 JSON Schema
	IsWriteOperation() bool
	Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error)
}

// Registry 技能注册表
type Registry struct {
	skills map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]Skill)}
}

func (r *Registry) Register(s Skill) {
	r.skills[s.Name()] = s
}

func (r *Registry) Get(name string) (Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

func (r *Registry) List() []Skill {
	var list []Skill
	for _, s := range r.skills {
		list = append(list, s)
	}
	return list
}
