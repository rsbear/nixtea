// file: internal/config/config.go

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	// Server configuration
	Host string
	Port string

	// SSH configuration
	HostKeyPath string

	// Database configuration
	DBDir  string
	DBName string
}

// New creates a new Config instance with values from environment variables
// and defaults where necessary
func NewCfg() (*Config, error) {
	hostKeyPath := os.Getenv("HOST_KEY_PATH")
	if hostKeyPath == "" {
		// Default from compile-time variable in main.go
		hostKeyPath = "/etc/nixtea/ssh/id_ed25519"
	}

	// Ensure the SSH directory exists
	sshDir := filepath.Dir(hostKeyPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create SSH directory: %w", err)
	}

	return &Config{
		// Server settings
		Host: getEnvOrDefault("HOST", "localhost"),
		Port: getEnvOrDefault("PORT", "23234"),

		// SSH settings
		HostKeyPath: hostKeyPath,

		// Database settings
		DBDir:  getEnvOrDefault("DB_DIR", "/tmp"),
		DBName: getEnvOrDefault("DB_NAME", "nixtea.db"),
	}, nil
}

// Helper function to get environment variable with default fallback
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		fmt.Printf("Using env value for %s: %s\n", key, value)
		return value
	}
	fmt.Printf("Using default value for %s: %s\n", key, defaultValue)
	return defaultValue
}
