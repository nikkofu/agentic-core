package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseModelOutput 尝试将模型原始输出解析为 ActionEnvelope
// 如果输出包含 Markdown 代码块 (```json ... ```)，它会自动剥离
func ParseModelOutput(raw string) (ActionEnvelope, error) {
	clean := strings.TrimSpace(raw)
	
	// 处理 Markdown 代码块
	if strings.HasPrefix(clean, "```json") {
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	} else if strings.HasPrefix(clean, "```") {
		clean = strings.TrimPrefix(clean, "```")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	}

	var action ActionEnvelope
	if err := json.Unmarshal([]byte(clean), &action); err != nil {
		return ActionEnvelope{}, fmt.Errorf("invalid_json_error: %v (raw: %s)", err, raw)
	}

	// 基础校验
	if action.Think == "" && action.Final == "" && action.CallSkill == nil {
		return ActionEnvelope{}, fmt.Errorf("invalid_action_error: response contains no think, final, or call_skill")
	}

	return action, nil
}
