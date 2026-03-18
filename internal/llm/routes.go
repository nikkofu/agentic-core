package llm

import (
	"fmt"
)

// StaticRoute 定义了逻辑别名到物理模型的映射
type StaticRoute struct {
	Alias         string `json:"alias"`
	Provider      string `json:"provider"`       // openai, deepseek, etc.
	BaseURL       string `json:"base_url"`       // 可选覆盖
	APIKeyRef     string `json:"api_key_ref"`    // 环境变量名或引用标识
	UpstreamModel string `json:"upstream_model"` // 实际请求给 Provider 的模型 ID
	TimeoutMs     int    `json:"timeout_ms"`     // 超时设置
}

// RouteTable 路由表
type RouteTable struct {
	routes map[string]StaticRoute
}

func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes: make(map[string]StaticRoute),
	}
}

func (t *RouteTable) Register(r StaticRoute) error {
	if r.Alias == "" {
		return fmt.Errorf("route alias cannot be empty")
	}
	t.routes[r.Alias] = r
	return nil
}

func (t *RouteTable) Get(alias string) (StaticRoute, bool) {
	r, ok := t.routes[alias]
	return r, ok
}
