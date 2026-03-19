package memory

import "testing"

func TestNormalizeTaskStatusPreservesKnownStates(t *testing.T) {
    for _, status := range []string{
        TaskStatusPending,
        TaskStatusRunning,
        TaskStatusSuccess,
        TaskStatusFailed,
        TaskStatusRejected,
        TaskStatusTimeout,
        TaskStatusCancelled,
    } {
        if got := NormalizeTaskStatus(status); got != status {
            t.Fatalf("expected %q to remain %q, got %q", status, status, got)
        }
    }
}

func TestNormalizeTaskStatusFallsBackToFailed(t *testing.T) {
    if got := NormalizeTaskStatus("completely-unknown-status"); got != TaskStatusFailed {
        t.Fatalf("unexpected fallback status: got %q, want %q", got, TaskStatusFailed)
    }

    if got := NormalizeTaskStatus("error"); got != TaskStatusFailed {
        t.Fatalf("unexpected fallback for %q: got %v, want %v", "error", got, TaskStatusFailed)
    }
}

func TestIsTerminalTaskStatus(t *testing.T) {
    cases := []struct {
        status string
        want   bool
    }{
        {TaskStatusPending, false},
        {TaskStatusRunning, false},
        {TaskStatusSuccess, true},
        {TaskStatusFailed, true},
        {TaskStatusRejected, true},
        {TaskStatusTimeout, true},
        {TaskStatusCancelled, true},
        {"unknown", true},
    }

    for _, tc := range cases {
        if got := IsTerminalTaskStatus(tc.status); got != tc.want {
            t.Fatalf("IsTerminalTaskStatus(%q) = %v, want %v", tc.status, got, tc.want)
        }
    }
}

func TestIsSuccessfulTaskStatus(t *testing.T) {
    if !IsSuccessfulTaskStatus(TaskStatusSuccess) {
        t.Fatalf("expected %q to be successful", TaskStatusSuccess)
    }

    for _, status := range []string{
        TaskStatusPending,
        TaskStatusRunning,
        TaskStatusFailed,
        TaskStatusRejected,
        TaskStatusTimeout,
        TaskStatusCancelled,
        "unknown",
    } {
        if IsSuccessfulTaskStatus(status) {
            t.Fatalf("expected %q not to be successful", status)
        }
    }
}
