package process

import (
	"encoding/json"
	"strings"
)

const redactedValue = "[REDACTED]"

func redactJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return raw
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}

	data, err := json.Marshal(redactValue(value))
	if err != nil {
		return raw
	}
	return data
}

func redactValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			if isSensitiveKey(key) {
				redacted[key] = redactedValue
				continue
			}
			redacted[key] = redactValue(nested)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			redacted = append(redacted, redactValue(item))
		}
		return redacted
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
	sensitiveFragments := []string{
		"apikey",
		"token",
		"secret",
		"password",
		"authorization",
		"credential",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func legacyStatus(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR":
		return "error"
	case "WARN", "WARNING":
		return "warning"
	default:
		return "ok"
	}
}
