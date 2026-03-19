package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var defaultConfigCandidates = func(workingDir string) []string {
	return []string{
		filepath.Join(workingDir, defaultConfigFileName),
		systemConfigPath,
	}
}

var statConfigPath = os.Stat

func resolveWorkingDir(workingDir string) (string, error) {
	if strings.TrimSpace(workingDir) != "" {
		return workingDir, nil
	}

	currentWorkingDir, err := getWorkingDir()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return currentWorkingDir, nil
}

func loadFileConfig(path string, explicitPath bool) (fileConfig, string, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		if explicitPath {
			return fileConfig{}, "", errors.New("config path is required")
		}
		return fileConfig{}, "", nil
	}

	// #nosec G304 -- the operator explicitly chooses the config file path via CLI or service config.
	data, err := os.ReadFile(trimmedPath)
	if err != nil {
		if !explicitPath && errors.Is(err, os.ErrNotExist) {
			return fileConfig{}, "", nil
		}
		return fileConfig{}, "", fmt.Errorf("read config: %w", err)
	}

	var raw fileConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return fileConfig{}, "", fmt.Errorf("decode config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fileConfig{}, "", errors.New("decode config: unexpected trailing JSON data")
		}
		return fileConfig{}, "", fmt.Errorf("decode config: %w", err)
	}

	return raw, filepath.Dir(trimmedPath), nil
}
