package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Agent         AgentConfig         `mapstructure:"agent"`
	Server        ServerConfig        `mapstructure:"server"`
	Work          WorkConfig          `mapstructure:"work"`
	Tasks         TasksConfig         `mapstructure:"tasks"`
	Heartbeat     HeartbeatConfig     `mapstructure:"heartbeat"`
	ScreenCapture ScreenCaptureConfig `mapstructure:"screen_capture"`
}

type AgentConfig struct {
	Name  string `mapstructure:"name"`
	Token string `mapstructure:"token"`
}

type ServerConfig struct {
	Address string `mapstructure:"address"`
}

type WorkConfig struct {
	Directory string `mapstructure:"directory"`
}

type TasksConfig struct {
	MaxConcurrent int32 `mapstructure:"max_concurrent"`
}

type HeartbeatConfig struct {
	IntervalSeconds int `mapstructure:"interval_seconds"`
}

type ScreenCaptureConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	IntervalSeconds int  `mapstructure:"interval_seconds"`
	Quality         int  `mapstructure:"quality"`
	MaxWidth        int  `mapstructure:"max_width"`
	MaxHeight       int  `mapstructure:"max_height"`
}

var GlobalConfig *Config

func Load(configPath string) (*Config, error) {
	viper.SetConfigType("yaml")

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// Look for config in common locations
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
		viper.AddConfigPath(filepath.Join(".", "solder"))
		home, _ := os.UserHomeDir()
		viper.AddConfigPath(filepath.Join(home, ".solder"))
	}

	// Set defaults
	viper.SetDefault("agent.name", "")
	viper.SetDefault("agent.token", "default-token")
	viper.SetDefault("server.address", "http://localhost:8080")
	viper.SetDefault("work.directory", "./work")
	viper.SetDefault("tasks.max_concurrent", 1)
	viper.SetDefault("heartbeat.interval_seconds", 30)
	viper.SetDefault("screen_capture.enabled", true)
	viper.SetDefault("screen_capture.interval_seconds", 30)
	viper.SetDefault("screen_capture.quality", 60)
	viper.SetDefault("screen_capture.max_width", 1280)
	viper.SetDefault("screen_capture.max_height", 720)

	// Read from environment variables (with priority)
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SOLDER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Allow environment variable overrides
	if addr := os.Getenv("MOTHERSHIP_ADDR"); addr != "" {
		viper.Set("server.address", addr)
	}
	if name := os.Getenv("RUNNER_NAME"); name != "" {
		viper.Set("agent.name", name)
	}
	if token := os.Getenv("RUNNER_TOKEN"); token != "" {
		viper.Set("agent.token", token)
	}
	if workDir := os.Getenv("WORK_DIR"); workDir != "" {
		viper.Set("work.directory", workDir)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	GlobalConfig = &cfg
	return &cfg, nil
}

