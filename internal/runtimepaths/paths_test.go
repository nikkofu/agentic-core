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

func TestResolveRuntimeRoot_OverrideRelativePathNormalizedToAbsolute(t *testing.T) {
	tmp := t.TempDir()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	got, err := ResolveRuntimeRoot("runtime", tmp)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot returned error: %v", err)
	}

	want, err := filepath.Abs("runtime")
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("expected absolute normalized override %q, got %q", want, got)
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

func TestResolveRuntimeRoot_EmptyCwdReturnsError(t *testing.T) {
	_, err := ResolveRuntimeRoot("", "")
	if err == nil {
		t.Fatal("expected error when cwd is empty")
	}
}

func TestResolveRuntimeRoot_NonExistentCwdReturnsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-dir")
	_, err := ResolveRuntimeRoot("", missing)
	if err == nil {
		t.Fatal("expected error for non-existent cwd")
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

func TestSQLiteDSNParentDirToPrepare_Matrix(t *testing.T) {
	absDB := filepath.Join(string(filepath.Separator), "tmp", "agentic-core", "state.db")

	tests := []struct {
		name      string
		dsn       string
		wantDir   string
		wantMkdir bool
	}{
		{
			name:      "memory literal has no mkdir",
			dsn:       ":memory:",
			wantDir:   "",
			wantMkdir: false,
		},
		{
			name:      "file dsn with mode memory has no mkdir",
			dsn:       "file:agentic?mode=memory&cache=shared",
			wantDir:   "",
			wantMkdir: false,
		},
		{
			name:      "file absolute path dsn prepares parent",
			dsn:       "file:/tmp/agentic-core/state.db",
			wantDir:   filepath.Dir(absDB),
			wantMkdir: true,
		},
		{
			name:      "plain absolute path prepares parent",
			dsn:       absDB,
			wantDir:   filepath.Dir(absDB),
			wantMkdir: true,
		},
		{
			name:      "plain relative path prepares parent",
			dsn:       filepath.Join("var", "state", "agent.db"),
			wantDir:   filepath.Join("var", "state"),
			wantMkdir: true,
		},
		{
			name:      "unparseable file dsn has no mkdir",
			dsn:       "file://%zz",
			wantDir:   "",
			wantMkdir: false,
		},
		{
			name:      "non filesystem scheme has no mkdir",
			dsn:       "postgres://localhost/agentic",
			wantDir:   "",
			wantMkdir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotMkdir := SQLiteDSNParentDirToPrepare(tt.dsn)
			if gotMkdir != tt.wantMkdir {
				t.Fatalf("expected mkdir=%v, got %v", tt.wantMkdir, gotMkdir)
			}
			if gotDir != tt.wantDir {
				t.Fatalf("expected dir %q, got %q", tt.wantDir, gotDir)
			}
		})
	}
}
