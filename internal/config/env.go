package config

import (
	"os"
	"path/filepath"
	"strings"
)

func applyEnvironment(raw fileConfig, env map[string]string, workingDir string) fileConfig {
	tokenPath, ok := lookupEnv(env, envProviderCloudflareAPITokenFile)
	if !ok {
		return raw
	}
	raw.Provider.Cloudflare.APITokenFile = resolvePath(tokenPath, workingDir)
	return raw
}

func lookupEnv(env map[string]string, key string) (string, bool) {
	if env != nil {
		value, ok := env[key]
		return value, ok
	}
	return os.LookupEnv(key)
}

func resolvePath(path string, baseDir string) string {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
