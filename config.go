// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Teal Bauer

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const defaultServer = "https://api.openshock.app"
const defaultUserAgent = "britzelator/0.1"

// FileConfig is the on-disk configuration. It only holds values the user
// explicitly stored; flag and environment lookups happen in globals.resolve.
type FileConfig struct {
	Token     string `json:"token,omitempty"`
	Session   string `json:"session,omitempty"`
	Server    string `json:"server,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func defaultConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "britzelator", "config.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "britzelator", "config.json")
}

// loadDotEnv reads KEY=VALUE lines from a .env-style file. A missing or
// unreadable file yields no values rather than an error, and `export `
// prefixes, blank lines, comments, and surrounding quotes are tolerated.
func loadDotEnv(path string) map[string]string {
	values := map[string]string{}
	if path == "" {
		return values
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}

func loadFileConfig(path string) (FileConfig, error) {
	var cfg FileConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveFileConfig(path string, cfg FileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}
