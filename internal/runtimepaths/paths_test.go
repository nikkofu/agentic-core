package runtimepaths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntimeRoot_OverrideWins(t *testing.T) {
	tmp := t.TempDir()
	override := filepath.Join(tmp, "runtime")

	got, err := ResolveRuntimeRoot(override, tmp)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot returned error: %v", err)
	}

	want, err := filepath.Abs(override)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("expected override path %q, got %q", want, got)
	}
}

func TestResolveRuntimeRoot_NearestAncestorGoMod(t *testing.T) {
	tmp := t.TempDir()

	repo := filepath.Join(tmp, "repo")
	nested := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	got, err := ResolveRuntimeRoot("", nested)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot returned error: %v", err)
	}

	want, err := filepath.Abs(repo)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("expected repo root %q, got %q", want, got)
	}
}

func TestResolveRuntimeRoot_FallbackToCwdWhenGoModAbsent(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "no-mod", "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}

	got, err := ResolveRuntimeRoot("", cwd)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot returned error: %v", err)
	}

	want, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("expected cwd fallback %q, got %q", want, got)
	}
}
