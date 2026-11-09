package config

import (
	"MyGateway/internal/clients"
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Port      string           `json:"port"`
	BaseURL   string           `json:"base_url"`
	StreamURL string           `json:"stream_url"`
	APIKeys   []clients.APIKey `json:"api_keys"`
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
	for _, key := range cfg.APIKeys {
		if key.Key == "" {
			return fmt.Errorf("API key is required")
		}
		if key.Name == "" {
			return fmt.Errorf("API key name is required")
		}
		for _, upstream := range key.Upstream {
			if upstream.BaseURL == "" {
				return fmt.Errorf("upstream base URL is required")
			}
			if upstream.Key == "" {
				return fmt.Errorf("upstream key is required")
			}
		}
	}
	return nil
}
