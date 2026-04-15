package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func storeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "four")
}

func ensureStoreDir() error {
	return os.MkdirAll(storeDir(), 0o755)
}

func historyDir() string {
	return filepath.Join(storeDir(), "history")
}

func ensureHistoryDir() error {
	return os.MkdirAll(historyDir(), 0o755)
}

func storePath(name string) string {
	return filepath.Join(storeDir(), name)
}

func loadJSON(name string, v interface{}) error {
	data, err := os.ReadFile(storePath(name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func saveJSON(name string, v interface{}) error {
	if err := ensureStoreDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storePath(name), data, 0o644)
}
