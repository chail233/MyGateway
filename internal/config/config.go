package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Port      string   `json:"port"`
	BaseURL   string   `json:"base_url"`
	StreamURL string   `json:"stream_url"`
	APIKeys   []APIKey `json:"api_keys"`
}

type APIKey struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	RPS   int    `json:"rps"`
	Burst int    `json:"burst"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return &cfg, nil
}

func (cfg *Config) Validate() error {
	if cfg.Port == "" {
		return fmt.Errorf("port is required")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	if cfg.StreamURL == "" {
		return fmt.Errorf("stream URL is required")
	}
	return nil
}
