package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Port    string `json:"port"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
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
