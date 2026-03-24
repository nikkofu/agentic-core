package runtimepaths

import (
	"errors"
	"os"
	"path/filepath"
)

func ResolveRuntimeRoot(override string, cwd string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}

	if cwd == "" {
		return "", errors.New("cwd is required when override is empty")
	}

	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	if root, ok, err := findNearestGoModAncestor(absCWD); err != nil {
		return "", err
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
