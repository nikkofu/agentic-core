package llm

import (
	"fmt"
	"strings"
)

// SystemPromptBuilder 负责构建复杂的系统提示词
type SystemPromptBuilder struct {
	role        string
	instructions []string
	tools       []ToolInfo
}

type ToolInfo struct {
	Name        string
	Description string
	Params      string // JSON Schema string
}

func NewSystemPromptBuilder(role string) *SystemPromptBuilder {
	return &SystemPromptBuilder{
		role: role,
	}
}

func (b *SystemPromptBuilder) AddInstruction(instr string) *SystemPromptBuilder {
	b.instructions = append(b.instructions, instr)
	return b
}

func (b *SystemPromptBuilder) AddTool(tool ToolInfo) *SystemPromptBuilder {
	b.tools = append(b.tools, tool)
	return b
}

func (b *SystemPromptBuilder) Build() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Role: %s\n\n", b.role))
	
	if len(b.instructions) > 0 {
		sb.WriteString("Instructions:\n")
		for i, instr := range b.instructions {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, instr))
		}
		sb.WriteString("\n")
	}

	if len(b.tools) > 0 {
		sb.WriteString("Available Tools:\n")
		for _, tool := range b.tools {
			sb.WriteString(fmt.Sprintf("- %s: %s (Params: %s)\n", tool.Name, tool.Description, tool.Params))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("You must strictly follow the response format required (JSON).")
	return sb.String()
}
