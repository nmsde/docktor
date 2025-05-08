package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// Config represents the application configuration
type Config struct {
	Fly struct {
		APIToken     string `yaml:"api_token,omitempty"`
		Organization string `yaml:"organization"`
		Region       string `yaml:"region"`
		Machine      struct {
			CPUKind  string `yaml:"cpu_kind"`
			CPUs     int    `yaml:"cpus"`
			MemoryMB int    `yaml:"memory_mb"`
		} `yaml:"machine"`
	} `yaml:"fly"`
}

func loadConfig() (*Config, error) {
	config := &Config{}

	// Load from .docktor file if it exists
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// First try current directory
		viper.AddConfigPath(".")
		// Then try home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		viper.AddConfigPath(home)
		
		viper.SetConfigType("yaml")
		viper.SetConfigName(".docktor")
	}

	// Load environment variables
	if apiToken := os.Getenv("FLY_API_TOKEN"); apiToken != "" {
		fmt.Printf("Found API token in environment (length: %d)\n", len(apiToken))
		config.Fly.APIToken = apiToken
	} else {
		fmt.Println("No API token found in environment variables")
	}

	if orgID := os.Getenv("FLY_ORG_ID"); orgID != "" {
		fmt.Printf("Found organization ID in environment: %s\n", orgID)
		config.Fly.Organization = orgID
	} else {
		fmt.Println("No organization ID found in environment variables")
	}

	// Try to load .docktor.env file
	envFile := ".docktor.env"
	if _, err := os.Stat(envFile); err == nil {
		fmt.Printf("Found .docktor.env file\n")
		file, err := os.Open(envFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open .docktor.env: %w", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "FLY_API_TOKEN=") {
				token := strings.TrimPrefix(line, "FLY_API_TOKEN=")
				fmt.Printf("Found API token in .docktor.env (length: %d)\n", len(token))
				config.Fly.APIToken = token
			}
			if strings.HasPrefix(line, "FLY_ORG_ID=") {
				orgID := strings.TrimPrefix(line, "FLY_ORG_ID=")
				fmt.Printf("Found organization ID in .docktor.env: %s\n", orgID)
				config.Fly.Organization = orgID
			}
		}
	} else {
		fmt.Printf("No .docktor.env file found: %v\n", err)
	}

	// Read YAML config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("Found config file: %s\n", viper.ConfigFileUsed())
		// Unmarshal config
		if err := viper.Unmarshal(config); err != nil {
			fmt.Printf("Warning: failed to unmarshal YAML config: %v\n", err)
		}
	} else {
		fmt.Printf("No YAML config file found: %v\n", err)
	}

	// Set default region if not specified
	if config.Fly.Region == "" {
		config.Fly.Region = "iad"
	}

	// Validate required fields
	if config.Fly.APIToken == "" {
		return nil, fmt.Errorf("FLY_API_TOKEN is required. Set it in .docktor.env or as an environment variable")
	}

	if config.Fly.Organization == "" {
		return nil, fmt.Errorf("FLY_ORG_ID is required. Set it in .docktor.env or as an environment variable")
	}

	fmt.Printf("Configuration loaded successfully with API token length: %d and organization: %s\n", len(config.Fly.APIToken), config.Fly.Organization)
	return config, nil
}

var rootCmd = &cobra.Command{
	Use:   "docktor",
	Short: "Docktor - Remote Docker image building and vulnerability scanning",
	Long: `Docktor is a CLI tool that helps you build and scan Docker images
remotely using Fly.io Machines. It provides a cost-effective way to
build and scan images without using local resources.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.docktor)")
}

func initConfig() {
	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		fmt.Println("Error binding flags:", err)
		os.Exit(1)
	}
} 