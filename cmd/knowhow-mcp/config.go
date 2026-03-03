package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Port      int        `toml:"port"`
	Instances []Instance `toml:"instance"`
}

type Instance struct {
	Name  string `toml:"name"`
	URL   string `toml:"url"`
	Token string `toml:"token"`
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		path = filepath.Join(home, ".config", "knowhow-mcp", "config.toml")
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}

	if len(cfg.Instances) == 0 {
		return nil, fmt.Errorf("config must have at least one [[instance]]")
	}

	for i, inst := range cfg.Instances {
		if inst.Name == "" {
			return nil, fmt.Errorf("instance %d: name is required", i)
		}
		if inst.URL == "" {
			return nil, fmt.Errorf("instance %q: url is required", inst.Name)
		}
		if inst.Token == "" {
			return nil, fmt.Errorf("instance %q: token is required", inst.Name)
		}
	}

	if cfg.Port == 0 {
		cfg.Port = 8585
	}

	return &cfg, nil
}
