package llm

import (
	"testing"
)

func TestParseModelOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantFinal string
		wantTool  string
		wantErr   bool
	}{
		{
			name:      "simple final output",
			input:     `{"think": "finished", "final": "hello world"}`,
			wantFinal: "hello world",
			wantErr:   false,
		},
		{
			name:      "markdown wrapped output",
			input:     "```json\n{\"think\": \"thinking\", \"final\": \"markdown reply\"}\n```",
			wantFinal: "markdown reply",
			wantErr:   false,
		},
		{
			name:      "tool call output",
			input:     `{"think": "need time", "call_skill": {"name": "get_time", "arguments": {}}}`,
			wantTool:  "get_time",
			wantErr:   false,
		},
		{
			name:      "invalid json",
			input:     `not a json`,
			wantErr:   true,
		},
		{
			name:      "empty envelope",
			input:     `{}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelOutput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseModelOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tt.wantFinal != "" && got.Final != tt.wantFinal {
					t.Errorf("ParseModelOutput() got final = %v, want %v", got.Final, tt.wantFinal)
				}
				if tt.wantTool != "" && (got.CallSkill == nil || got.CallSkill.Name != tt.wantTool) {
					t.Errorf("ParseModelOutput() got tool = %v, want %v", got.CallSkill, tt.wantTool)
				}
			}
		})
	}
}
