package runtimepaths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
