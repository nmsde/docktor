package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Docktor configuration",
	Long: `Initialize Docktor configuration by creating a .docktor.env file in the current directory.
This file will contain your Google Cloud configuration and service account details.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if .docktor.env already exists
		if _, err := os.Stat(".docktor.env"); err == nil {
			return fmt.Errorf(".docktor.env already exists")
		}

		// Get Google Cloud project ID
		fmt.Print("Enter your Google Cloud project ID: ")
		var gcpProjectID string
		fmt.Scanln(&gcpProjectID)
		if gcpProjectID == "" {
			return fmt.Errorf("Google Cloud project ID is required")
		}

		// Get Google Cloud region
		fmt.Print("Enter your Google Cloud region (default: global): ")
		var gcpRegion string
		fmt.Scanln(&gcpRegion)
		if gcpRegion == "" {
			gcpRegion = "global"
		}

		// Ask about service account configuration
		fmt.Print("Do you want to use a service account key file? (y/n): ")
		var useServiceAccount string
		fmt.Scanln(&useServiceAccount)

		var gcpServiceAccount, gcpServiceKeyPath string
		if useServiceAccount == "y" || useServiceAccount == "Y" {
			// Get service account email
			fmt.Print("Enter your Google Cloud service account email: ")
			fmt.Scanln(&gcpServiceAccount)
			if gcpServiceAccount == "" {
				return fmt.Errorf("service account email is required")
			}

			// Get service account key file path
			fmt.Print("Enter the path to your service account key file: ")
			fmt.Scanln(&gcpServiceKeyPath)
			if gcpServiceKeyPath == "" {
				return fmt.Errorf("service account key file path is required")
			}

			// Verify the key file exists
			if _, err := os.Stat(gcpServiceKeyPath); os.IsNotExist(err) {
				return fmt.Errorf("service account key file not found at: %s", gcpServiceKeyPath)
			}
		}

		// Create .docktor.env file
		envContent := fmt.Sprintf(`GCP_PROJECT_ID=%s
GCP_REGION=%s
`, gcpProjectID, gcpRegion)

		// Add service account configuration if provided
		if gcpServiceAccount != "" && gcpServiceKeyPath != "" {
			envContent += fmt.Sprintf(`GCP_SERVICE_ACCOUNT=%s
GCP_SERVICE_KEY_PATH=%s
`, gcpServiceAccount, gcpServiceKeyPath)
		}

		if err := os.WriteFile(".docktor.env", []byte(envContent), 0600); err != nil {
			return fmt.Errorf("failed to write .docktor.env: %w", err)
		}

		fmt.Println("\nConfiguration initialized successfully!")
		if gcpServiceAccount == "" {
			fmt.Println("\nNote: No service account configured. Make sure to set the GOOGLE_APPLICATION_CREDENTIALS environment variable before running docktor scan.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
} 