package memory

import "strings"

const (
    TaskStatusPending   = "pending"
    TaskStatusRunning   = "running"
    TaskStatusSuccess   = "success"
    TaskStatusFailed    = "failed"
    TaskStatusRejected  = "rejected"
    TaskStatusTimeout   = "timeout"
    TaskStatusCancelled = "cancelled"
)

func NormalizeTaskStatus(status string) string {
    normalized := strings.ToLower(strings.TrimSpace(status))
    switch normalized {
    case TaskStatusPending,
        TaskStatusRunning,
        TaskStatusSuccess,
        TaskStatusFailed,
        TaskStatusRejected,
        TaskStatusTimeout,
        TaskStatusCancelled:
        return normalized
    default:
        return TaskStatusFailed
    }
}

func IsTerminalTaskStatus(status string) bool {
    switch NormalizeTaskStatus(status) {
    case TaskStatusPending,
        TaskStatusRunning:
        return false
    default:
        return true
    }
}

func IsSuccessfulTaskStatus(status string) bool {
    return NormalizeTaskStatus(status) == TaskStatusSuccess
}
