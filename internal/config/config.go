package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	// Google Cloud Build configuration
	GCPProjectID        string `mapstructure:"gcp_project_id"`
	GCPRegion          string `mapstructure:"gcp_region"`
	GCPServiceAccount  string `mapstructure:"gcp_service_account"`
	GCPServiceKeyPath  string `mapstructure:"gcp_service_key_path"`

	// Scanner configuration
	Severity string `mapstructure:"severity"`
	Timeout  int    `mapstructure:"timeout"`
}

func LoadConfig() (*Config, error) {
	// First, try to load from .docktor.env in the current directory
	if err := loadEnvFile(".docktor.env"); err != nil {
		return nil, fmt.Errorf("failed to load .docktor.env: %w", err)
	}

	// Then, try to load from .docktor.yaml in the current directory
	if err := loadYamlFile(".docktor.yaml"); err != nil {
		return nil, fmt.Errorf("failed to load .docktor.yaml: %w", err)
	}

	// Finally, try to load from ~/.docktor.yaml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	if err := loadYamlFile(filepath.Join(homeDir, ".docktor.yaml")); err != nil {
		return nil, fmt.Errorf("failed to load ~/.docktor.yaml: %w", err)
	}

	config := &Config{}
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if config.GCPProjectID == "" {
		return nil, fmt.Errorf("gcp_project_id is required")
	}

	// Validate GCP service account configuration
	if config.GCPServiceAccount == "" && config.GCPServiceKeyPath == "" {
		// Check for GOOGLE_APPLICATION_CREDENTIALS environment variable
		if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
			return nil, fmt.Errorf("either gcp_service_account or gcp_service_key_path must be set, or GOOGLE_APPLICATION_CREDENTIALS environment variable must be set")
		}
	}

	return config, nil
}

func loadEnvFile(filename string) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	}

	viper.SetConfigFile(filename)
	viper.SetConfigType("env")
	if err := viper.MergeInConfig(); err != nil {
		return fmt.Errorf("failed to read env file: %w", err)
	}

	return nil
}

func loadYamlFile(filename string) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	}

	viper.SetConfigFile(filename)
	viper.SetConfigType("yaml")
	if err := viper.MergeInConfig(); err != nil {
		return fmt.Errorf("failed to read yaml file: %w", err)
	}

	return nil
} 