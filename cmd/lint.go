package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/nmsde/docktor/internal/config"
	"github.com/nmsde/docktor/internal/gcp"
	"github.com/spf13/cobra"
)

var (
	lintFile    string
	lintContext string
)

var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Lint a Dockerfile using Hadolint",
	Long: `Lint a Dockerfile using Hadolint in Google Cloud Build.
This command uploads your Dockerfile to Cloud Build and runs Hadolint to check for best practices and common issues.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Initialize GCP client
		client, err := gcp.NewClient(cfg.GCPProjectID, cfg.GCPServiceAccount, cfg.GCPServiceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to create GCP client: %w", err)
		}

		// Get absolute path for context
		absContext, err := filepath.Abs(lintContext)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for context: %w", err)
		}

		// Lint the Dockerfile
		result, err := client.LintDockerfile(cmd.Context(), absContext, lintFile)
		if err != nil {
			return fmt.Errorf("failed to lint Dockerfile: %w", err)
		}

		// Print the linting results
		if len(result.Issues) == 0 {
			fmt.Println("✅ No issues found in your Dockerfile!")
		} else {
			fmt.Printf("\n🔍 Found %d issues in your Dockerfile:\n\n", len(result.Issues))
			for _, issue := range result.Issues {
				fmt.Printf("Line %d: %s\n", issue.Line, issue.Message)
				fmt.Printf("Level: %s\n", issue.Level)
				fmt.Printf("Code: %s\n\n", issue.Code)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(lintCmd)

	lintCmd.Flags().StringVarP(&lintFile, "file", "f", "Dockerfile", "Path to the Dockerfile")
	lintCmd.Flags().StringVarP(&lintContext, "context", "c", ".", "Path to the build context")
} 