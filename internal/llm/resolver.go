package llm

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// ModelResolver 负责管理多个 LLM 提供者和别名路由
type ModelResolver struct {
	mu        sync.RWMutex
	providers map[string]Provider
	routes    map[string]StaticRoute
}

func NewModelResolver() *ModelResolver {
	return &ModelResolver{
		providers: make(map[string]Provider),
		routes:    make(map[string]StaticRoute),
	}
}

// Register 注册一个提供者
func (r *ModelResolver) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// RegisterRoute 注册模型别名路由
func (r *ModelResolver) RegisterRoute(route StaticRoute) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[route.Alias] = route
}

// Get 获取并返回提供者
func (r *ModelResolver) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// ResolveByAlias 解析别名到物理提供者和路由配置
func (r *ModelResolver) ResolveByAlias(alias string) (Provider, StaticRoute, error) {
	r.mu.RLock()
	route, ok := r.routes[alias]
	r.mu.RUnlock()

	if !ok {
		return nil, StaticRoute{}, fmt.Errorf("invalid_request_error: model alias %s not found", alias)
	}

	p, ok := r.Get(route.Provider)
	if !ok {
		return nil, route, fmt.Errorf("invalid_request_error: provider %s (for alias %s) not found", route.Provider, alias)
	}

	return p, route, nil
}

// Resolve 根据名称获取提供者 (返回 error)
func (r *ModelResolver) Resolve(name string) (Provider, error) {
	p, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("llm provider %s not found", name)
	}
	return p, nil
}

// Predict 路由预测请求
func (r *ModelResolver) Predict(ctx context.Context, providerName string, req InferenceRequest) (string, error) {
	p, err := r.Resolve(providerName)
	if err != nil {
		return "", err
	}
	return p.Predict(ctx, req)
}

// PredictStream 路由流式预测请求
func (r *ModelResolver) PredictStream(ctx context.Context, providerName string, req InferenceRequest) (io.ReadCloser, error) {
	p, err := r.Resolve(providerName)
	if err != nil {
		return nil, err
	}
	return p.PredictStream(ctx, req)
}
