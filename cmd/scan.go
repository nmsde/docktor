package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nmsde/docktor/internal/config"
	"github.com/nmsde/docktor/internal/gcp"
	"github.com/spf13/cobra"
)

var (
	contextPath    string
	dockerfilePath string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a Docker image for vulnerabilities",
	Long: `Scan a Docker image for vulnerabilities using Google Cloud Build.
The image will be built and scanned in the cloud, and the results will be displayed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Initialize Google Cloud Build client
		client, err := gcp.NewClient(cfg.GCPProjectID, cfg.GCPServiceAccount, cfg.GCPServiceKeyPath)
		if err != nil {
			return fmt.Errorf("failed to initialize Google Cloud Build client: %w", err)
		}

		// Get absolute paths
		absContextPath, err := filepath.Abs(contextPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for context: %w", err)
		}

		absDockerfilePath, err := filepath.Abs(dockerfilePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for Dockerfile: %w", err)
		}

		// Start build and scan
		fmt.Println("Starting build and scan...")
		result, err := client.BuildAndScanImage(context.Background(), absContextPath, absDockerfilePath)
		if err != nil {
			return fmt.Errorf("failed to build and scan image: %w", err)
		}

		// Print results
		fmt.Printf("\nBuild completed with status: %s\n", result.Status)
		fmt.Printf("Build ID: %s\n", result.ID)
		fmt.Printf("Start time: %s\n", result.StartTime)
		fmt.Printf("End time: %s\n", result.EndTime)
		fmt.Printf("Logs: %s\n", result.Logs)

		// Cleanup
		if err := client.Cleanup(context.Background(), result.ID); err != nil {
			fmt.Printf("Warning: failed to cleanup build artifacts: %v\n", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVarP(&contextPath, "context", "c", ".", "Path to the build context")
	scanCmd.Flags().StringVarP(&dockerfilePath, "file", "f", "Dockerfile", "Path to the Dockerfile")
} 