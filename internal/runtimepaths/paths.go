package runtimepaths

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func ResolveRuntimeRoot(override string, cwd string) (string, error) {
	if override != "" {
		absOverride, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve runtime root from override: %w", err)
		}
		return absOverride, nil
	}

	if cwd == "" {
		return "", errors.New("cwd is required when override is empty")
	}

	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve runtime root from cwd: %w", err)
	}

	info, err := os.Stat(absCWD)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("cwd does not exist: %s", absCWD)
		}
		return "", fmt.Errorf("stat cwd %s: %w", absCWD, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd is not a directory: %s", absCWD)
	}

	if root, ok, err := findNearestGoModAncestor(absCWD); err != nil {
		return "", fmt.Errorf("find nearest go.mod ancestor from %s: %w", absCWD, err)
	} else if ok {
		return root, nil
	}

	return absCWD, nil
}

func SQLiteDSNParentDirToPrepare(dsn string) (string, bool) {
	if dsn == "" || dsn == ":memory:" {
		return "", false
	}

	if strings.HasPrefix(strings.ToLower(dsn), "file:") {
		return sqliteFileDSNParentDirToPrepare(dsn)
	}

	if dsnLooksLikeURIWithScheme(dsn) {
		return "", false
	}

	return filepath.Dir(dsn), true
}

func sqliteFileDSNParentDirToPrepare(dsn string) (string, bool) {
	u, err := url.Parse(dsn)
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}

	if strings.EqualFold(u.Query().Get("mode"), "memory") {
		return "", false
	}

	dbPath := u.Path
	if dbPath == "" {
		dbPath = u.Opaque
	}
	if dbPath == "" || dbPath == ":memory:" {
		return "", false
	}

	return filepath.Dir(dbPath), true
}

func dsnLooksLikeURIWithScheme(dsn string) bool {
	if len(dsn) >= 3 && ((dsn[0] >= 'A' && dsn[0] <= 'Z') || (dsn[0] >= 'a' && dsn[0] <= 'z')) && dsn[1] == ':' && (dsn[2] == '\\' || dsn[2] == '/') {
		return false
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return false
	}
	if len(u.Scheme) == 1 && (strings.HasPrefix(u.Path, `\`) || strings.HasPrefix(u.Opaque, `\`)) {
		return false
	}
	return u.Scheme != ""
}

func findNearestGoModAncestor(start string) (string, bool, error) {
	current := start
	for {
		goModPath := filepath.Join(current, "go.mod")
		info, err := os.Stat(goModPath)
		if err == nil && !info.IsDir() {
			return current, true, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		current = parent
	}
}
