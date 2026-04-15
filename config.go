package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	SourceFile       string                   `json:"source_file"`
	ShowSectors      bool                     `json:"show_sectors"`
	ShowTopPayers    bool                     `json:"show_top_payers"`
	ShowHoldings     bool                     `json:"show_holdings"`
	ShowGoal         bool                     `json:"show_goal"`
	SortBy           string                   `json:"sort_by"`
	SortDesc         bool                     `json:"sort_desc"`
	BucketGoals      map[BucketType]BucketGoal `json:"bucket_goals,omitempty"`
	YahooRefreshSecs int                      `json:"yahoo_refresh_secs,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		ShowSectors:   true,
		ShowTopPayers: true,
		ShowHoldings:  true,
		ShowGoal:      true,
		SortBy:        "symbol",
		SortDesc:      false,
	}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "four", "config.json")
}

func LoadConfig() *Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, cfg)
	return cfg
}

func (c *Config) Save() error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
