package gcp

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/sabhiram/go-gitignore"
	"google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/option"
)

type Client struct {
	buildService  *cloudbuild.Service
	storageClient *storage.Client
	projectID     string
}

type BuildResult struct {
	ID        string
	Status    string
	StartTime time.Time
	EndTime   time.Time
	Logs      string
	ScanResults *ScanResults
}

type ScanResults struct {
	Vulnerabilities []Vulnerability
}

type Vulnerability struct {
	VulnerabilityID string
	PkgName         string
	InstalledVersion string
	FixedVersion    string
	Severity        string
	Title           string
	Description     string
}

func NewClient(projectID, serviceAccount, serviceKeyPath string) (*Client, error) {
	ctx := context.Background()

	// Configure authentication options
	var opts []option.ClientOption

	// If service account and key path are provided, use them
	if serviceAccount != "" && serviceKeyPath != "" {
		// Read the service account key file
		keyFile, err := os.ReadFile(serviceKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read service account key file: %w", err)
		}

		// Parse the key file to verify it's valid JSON
		var keyData map[string]interface{}
		if err := json.Unmarshal(keyFile, &keyData); err != nil {
			return nil, fmt.Errorf("invalid service account key file: %w", err)
		}

		// Add the credentials option
		opts = append(opts, option.WithCredentialsFile(serviceKeyPath))
	}

	// Initialize Cloud Build service
	buildService, err := cloudbuild.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Build service: %w", err)
	}

	// Initialize Cloud Storage client
	storageClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Storage client: %w", err)
	}

	return &Client{
		buildService:  buildService,
		storageClient: storageClient,
		projectID:    projectID,
	}, nil
}

func (c *Client) BuildAndScanImage(ctx context.Context, contextPath, dockerfilePath string) (*BuildResult, error) {
	color.Blue("\n🚀 Starting build and scan process...")

	// Create a unique build ID
	buildID := fmt.Sprintf("docktor-%d", time.Now().Unix())
	color.Cyan("📌 Build ID: %s", buildID)

	// Upload build context to Cloud Storage
	bucketName := fmt.Sprintf("%s-docktor-builds", c.projectID)
	bucket := c.storageClient.Bucket(bucketName)

	// Create bucket if it doesn't exist
	if _, err := bucket.Attrs(ctx); err != nil {
		color.Yellow("📦 Creating new bucket: %s", bucketName)
		if err := bucket.Create(ctx, c.projectID, nil); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	// Upload build context
	if err := createAndUploadContext(ctx, c.storageClient, bucketName, buildID, contextPath); err != nil {
		return nil, fmt.Errorf("failed to upload build context: %w", err)
	}

	// Get the Dockerfile path relative to the context
	dockerfileArg := "Dockerfile"
	if dockerfilePath != "" {
		// Convert to absolute path if it's not already
		absDockerfilePath := dockerfilePath
		if !filepath.IsAbs(dockerfilePath) {
			absDockerfilePath = filepath.Join(contextPath, dockerfilePath)
		}

		// Get the path relative to the context
		relPath, err := filepath.Rel(contextPath, absDockerfilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to get relative path for Dockerfile: %w", err)
		}
		dockerfileArg = relPath
	}
	color.Cyan("📄 Using Dockerfile: %s", dockerfileArg)

	// Create build request
	color.Blue("\n🔨 Creating build request...")
	build := &cloudbuild.Build{
		Steps: []*cloudbuild.BuildStep{
			{
				Name: "gcr.io/cloud-builders/docker",
				Args: []string{
					"build",
					"-t", fmt.Sprintf("gcr.io/%s/%s", c.projectID, buildID),
					"-f", dockerfileArg,
					".",
				},
				Dir: "/workspace",
			},
			{
				Name: "aquasec/trivy",
				Args: []string{
					"image",
					"--format", "json",
					"--output", "/workspace/scan-results.json",
					fmt.Sprintf("gcr.io/%s/%s", c.projectID, buildID),
				},
			},
		},
		Timeout: "1800s", // 30 minutes
		Source: &cloudbuild.Source{
			StorageSource: &cloudbuild.StorageSource{
				Bucket: bucketName,
				Object: fmt.Sprintf("%s/context.tar.gz", buildID),
			},
		},
		Artifacts: &cloudbuild.Artifacts{
			Objects: &cloudbuild.ArtifactObjects{
				Location: fmt.Sprintf("gs://%s/%s", bucketName, buildID),
				Paths:    []string{"scan-results.json"},
			},
		},
	}

	// Create the build
	color.Blue("\n🚀 Starting build...")
	operation, err := c.buildService.Projects.Builds.Create(c.projectID, build).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create build: %w", err)
	}

	color.Cyan("⏳ Waiting for build to complete...")
	// Wait for build completion
	for {
		op, err := c.buildService.Operations.Get(operation.Name).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to get operation status: %w", err)
		}

		if op.Done {
			color.Green("✅ Build completed!")

			// Check if the scan results file exists
			color.Blue("\n🔍 Retrieving scan results...")
			scanResults, err := c.getScanResults(ctx, bucketName, buildID)
			if err != nil {
				// If we can't get the scan results, the build might have failed
				return nil, fmt.Errorf("build completed but failed to get scan results: %w", err)
			}

			// Get build start and end times from the operation
			startTime := time.Now()
			finishTime := time.Now()

			// Parse the operation metadata to get timing information
			if op.Metadata != nil {
				var metadata struct {
					Build struct {
						StartTime string `json:"startTime"`
						EndTime   string `json:"endTime"`
					} `json:"build"`
				}
				if err := json.Unmarshal(op.Metadata, &metadata); err == nil {
					if metadata.Build.StartTime != "" {
						startTime, _ = time.Parse(time.RFC3339, metadata.Build.StartTime)
					}
					if metadata.Build.EndTime != "" {
						finishTime, _ = time.Parse(time.RFC3339, metadata.Build.EndTime)
					}
				}
			}

			color.Green("✨ Scan completed successfully!")
			color.Cyan("📊 Found %d vulnerabilities", len(scanResults.Vulnerabilities))

			return &BuildResult{
				ID:          buildID,
				Status:      "SUCCESS",
				StartTime:   startTime,
				EndTime:     finishTime,
				Logs:        fmt.Sprintf("https://console.cloud.google.com/cloud-build/builds/%s?project=%s", buildID, c.projectID),
				ScanResults: scanResults,
			}, nil
		}

		time.Sleep(5 * time.Second)
	}
}

func (c *Client) getScanResults(ctx context.Context, bucketName, buildID string) (*ScanResults, error) {
	// Get the scan results file from Cloud Storage
	bucket := c.storageClient.Bucket(bucketName)
	obj := bucket.Object(fmt.Sprintf("%s/scan-results.json", buildID))

	// Check if the file exists
	_, err := obj.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan results file not found: %w", err)
	}

	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read scan results: %w", err)
	}
	defer reader.Close()

	// Read the entire content into a buffer
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read scan results content: %w", err)
	}

	// Create docktor directory if it doesn't exist
	docktorDir := "docktor"
	if err := os.MkdirAll(docktorDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create docktor directory: %w", err)
	}

	// Save the raw JSON file
	rawJSONPath := filepath.Join(docktorDir, fmt.Sprintf("%s-raw.json", buildID))
	if err := os.WriteFile(rawJSONPath, content, 0644); err != nil {
		return nil, fmt.Errorf("failed to save raw JSON file: %w", err)
	}

	// Parse the scan results
	var results struct {
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID  string `json:"VulnerabilityID"`
				PkgName         string `json:"PkgName"`
				InstalledVersion string `json:"InstalledVersion"`
				FixedVersion    string `json:"FixedVersion"`
				Severity        string `json:"Severity"`
				Title           string `json:"Title"`
				Description     string `json:"Description"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}

	if err := json.Unmarshal(content, &results); err != nil {
		return nil, fmt.Errorf("failed to parse scan results: %w", err)
	}

	// Convert to our ScanResults type
	scanResults := &ScanResults{
		Vulnerabilities: make([]Vulnerability, 0),
	}

	for _, result := range results.Results {
		for _, vuln := range result.Vulnerabilities {
			scanResults.Vulnerabilities = append(scanResults.Vulnerabilities, Vulnerability{
				VulnerabilityID:  vuln.VulnerabilityID,
				PkgName:         vuln.PkgName,
				InstalledVersion: vuln.InstalledVersion,
				FixedVersion:    vuln.FixedVersion,
				Severity:        vuln.Severity,
				Title:           vuln.Title,
				Description:     vuln.Description,
			})
		}
	}

	// Group vulnerabilities by severity
	vulnsBySeverity := make(map[string][]Vulnerability)
	for _, vuln := range scanResults.Vulnerabilities {
		vulnsBySeverity[vuln.Severity] = append(vulnsBySeverity[vuln.Severity], vuln)
	}

	// Generate HTML report
	htmlPath := filepath.Join(docktorDir, fmt.Sprintf("%s-report.html", buildID))
	htmlFile, err := os.Create(htmlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTML report: %w", err)
	}
	defer htmlFile.Close()

	// Write HTML header with styles
	fmt.Fprintf(htmlFile, `<!DOCTYPE html>
<html>
<head>
    <title>Vulnerability Scan Report - %s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
            padding: 20px;
            background: #f8f9fa;
            border-radius: 8px;
        }
        .summary {
            display: flex;
            justify-content: space-around;
            margin-bottom: 30px;
            flex-wrap: wrap;
        }
        .severity-card {
            padding: 15px;
            border-radius: 8px;
            margin: 10px;
            min-width: 200px;
            text-align: center;
            color: white;
        }
        .critical { background-color: #dc3545; }
        .high { background-color: #fd7e14; }
        .medium { background-color: #ffc107; color: #000; }
        .low { background-color: #20c997; }
        .vulnerability {
            margin-bottom: 20px;
            padding: 20px;
            border-radius: 8px;
            background: #f8f9fa;
        }
        .vulnerability.critical { border-left: 5px solid #dc3545; }
        .vulnerability.high { border-left: 5px solid #fd7e14; }
        .vulnerability.medium { border-left: 5px solid #ffc107; }
        .vulnerability.low { border-left: 5px solid #20c997; }
        .vulnerability h3 {
            margin-top: 0;
            color: #495057;
        }
        .vulnerability p {
            margin: 5px 0;
        }
        .label {
            font-weight: bold;
            color: #495057;
        }
        .timestamp {
            color: #6c757d;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>Vulnerability Scan Report</h1>
        <p class="timestamp">Generated on %s</p>
    </div>

    <div class="summary">
`, buildID, time.Now().Format("January 2, 2006 15:04:05"))

	// Write severity summary
	severityOrder := []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}
	for _, severity := range severityOrder {
		if vulns, ok := vulnsBySeverity[severity]; ok {
			fmt.Fprintf(htmlFile, `
        <div class="severity-card %s">
            <h2>%s</h2>
            <p>%d vulnerabilities</p>
        </div>`, strings.ToLower(severity), severity, len(vulns))
		}
	}

	fmt.Fprintf(htmlFile, `
    </div>

    <div class="vulnerabilities">`)

	// Write detailed findings
	for _, severity := range severityOrder {
		if vulns, ok := vulnsBySeverity[severity]; ok {
			for _, vuln := range vulns {
				fmt.Fprintf(htmlFile, `
        <div class="vulnerability %s">
            <h3>%s</h3>
            <p><span class="label">Package:</span> %s</p>
            <p><span class="label">Installed Version:</span> %s</p>`, 
					strings.ToLower(severity), vuln.Title, vuln.PkgName, vuln.InstalledVersion)

				if vuln.FixedVersion != "" {
					fmt.Fprintf(htmlFile, `
            <p><span class="label">Fixed Version:</span> %s</p>`, vuln.FixedVersion)
				}

				fmt.Fprintf(htmlFile, `
            <p><span class="label">Description:</span> %s</p>
            <p><span class="label">Vulnerability ID:</span> %s</p>
        </div>`, vuln.Description, vuln.VulnerabilityID)
			}
		}
	}

	fmt.Fprintf(htmlFile, `
    </div>
</body>
</html>`)

	// Print the path to the HTML report
	fmt.Printf("\nScan completed! View the results at: %s\n", htmlPath)

	// Open the report in the default browser
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		fmt.Printf("Warning: Could not get absolute path for report: %v\n", err)
		return scanResults, nil
	}

	// Convert path to URL format
	fileURL := fmt.Sprintf("file://%s", absPath)

	// Open browser based on OS
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", fileURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", fileURL)
	default: // "linux", "freebsd", etc.
		cmd = exec.Command("xdg-open", fileURL)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Warning: Could not open report in browser: %v\n", err)
	}

	return scanResults, nil
}

func (c *Client) Cleanup(ctx context.Context, buildID string) error {
	// Delete the build context from Cloud Storage
	bucketName := fmt.Sprintf("%s-docktor-builds", c.projectID)
	bucket := c.storageClient.Bucket(bucketName)
	object := bucket.Object(fmt.Sprintf("%s/context.tar.gz", buildID))
	
	if err := object.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete build context: %w", err)
	}

	return nil
}

func (c *Client) StartBuild(ctx context.Context, buildContext, dockerfilePath string) (*BuildResult, error) {
	// Create a unique build ID
	buildID := fmt.Sprintf("docktor-%s", uuid.New().String())

	// Create build request
	build := &cloudbuild.Build{
		Steps: []*cloudbuild.BuildStep{
			{
				Name: "gcr.io/cloud-builders/docker",
				Args: []string{
					"build",
					"-t", fmt.Sprintf("gcr.io/%s/%s", c.projectID, buildID),
					"-f", dockerfilePath,
					".",
				},
			},
		},
		Timeout: "1800s", // 30 minutes
	}

	// Start the build
	operation, err := c.buildService.Projects.Builds.Create(c.projectID, build).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to start build: %w", err)
	}

	// Wait for build to complete
	for {
		build, err := c.buildService.Projects.Builds.Get(c.projectID, operation.Name).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to get build status: %w", err)
		}

		if build.Status == "SUCCESS" {
			startTime, _ := time.Parse(time.RFC3339, build.StartTime)
			finishTime, _ := time.Parse(time.RFC3339, build.FinishTime)
			return &BuildResult{
				ID:        build.Id,
				Status:    build.Status,
				StartTime: startTime,
				EndTime:   finishTime,
				Logs:      build.LogUrl,
			}, nil
		} else if build.Status == "FAILURE" || build.Status == "TIMEOUT" || build.Status == "CANCELLED" {
			return nil, fmt.Errorf("build failed with status: %s", build.Status)
		}

		time.Sleep(5 * time.Second)
	}
}

func createAndUploadContext(ctx context.Context, storageClient *storage.Client, bucketName, buildID, contextPath string) error {
	color.Blue("📦 Preparing build context...")

	// Create a temporary file for the tar archive
	tmpFile, err := os.CreateTemp("", "docktor-context-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	color.Cyan("📝 Creating archive...")

	// Create gzip writer
	gzipWriter := gzip.NewWriter(tmpFile)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Load all .gitignore files
	color.Cyan("🔍 Loading .gitignore patterns...")
	gitignore, err := loadGitignore(contextPath)
	if err != nil {
		return fmt.Errorf("failed to load .gitignore patterns: %w", err)
	}

	// Function to check if a path should be excluded
	shouldExclude := func(path string) bool {
		// Get the base name of the path
		base := filepath.Base(path)
		
		// Exclude node_modules directories
		if strings.Contains(path, "/node_modules/") || strings.HasSuffix(path, "/node_modules") {
			return true
		}

		// Exclude hidden directories and files (starting with .)
		if strings.HasPrefix(base, ".") {
			return true
		}

		// Exclude .git directory
		if base == ".git" {
			return true
		}

		// Check against .gitignore patterns
		if gitignore != nil && gitignore.MatchesPath(path) {
			return true
		}

		return false
	}

	// Counters for statistics
	var totalFiles, excludedFiles int64
	var totalSize, excludedSize int64

	// Walk through the directory
	err = filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == contextPath {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(contextPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip if path should be excluded
		if shouldExclude(relPath) {
			if info.IsDir() {
				color.Yellow("  ⏩ Skipping directory: %s", relPath)
				return filepath.SkipDir
			}
			excludedFiles++
			excludedSize += info.Size()
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// If it's a regular file, write its contents
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file to tar: %w", err)
			}

			totalFiles++
			totalSize += info.Size()

			// Print progress for large files
			if info.Size() > 1024*1024 { // 1MB
				color.Green("  📄 Adding file: %s (%s)", relPath, formatSize(info.Size()))
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	// Close writers to ensure all data is written
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Print statistics
	color.Cyan("\n📊 Build Context Statistics:")
	color.White("  Total files included: %d (%s)", totalFiles, formatSize(totalSize))
	color.Yellow("  Files excluded: %d (%s)", excludedFiles, formatSize(excludedSize))
	color.Green("  Final archive size: %s", formatSize(getFileSize(tmpFile)))

	// Reset file pointer to beginning
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset file pointer: %w", err)
	}

	color.Blue("\n☁️  Uploading to Cloud Storage...")

	// Upload to Cloud Storage
	bucket := storageClient.Bucket(bucketName)
	obj := bucket.Object(fmt.Sprintf("%s/context.tar.gz", buildID))
	writer := obj.NewWriter(ctx)

	if _, err := io.Copy(writer, tmpFile); err != nil {
		return fmt.Errorf("failed to upload to Cloud Storage: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close Cloud Storage writer: %w", err)
	}

	color.Green("✅ Build context uploaded successfully!")

	return nil
}

// loadGitignore loads all .gitignore files in the project
func loadGitignore(rootPath string) (*ignore.GitIgnore, error) {
	// Find all .gitignore files
	var gitignoreFiles []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == ".gitignore" {
			gitignoreFiles = append(gitignoreFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find .gitignore files: %w", err)
	}

	if len(gitignoreFiles) == 0 {
		return nil, nil
	}

	// Read all .gitignore files
	var patterns []string
	for _, file := range gitignoreFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read .gitignore file %s: %w", file, err)
		}

		// Get the directory of this .gitignore file relative to root
		dir, err := filepath.Rel(rootPath, filepath.Dir(file))
		if err != nil {
			return nil, fmt.Errorf("failed to get relative path for .gitignore: %w", err)
		}

		// Add patterns with their directory prefix
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if dir != "." {
				// Prefix the pattern with the directory
				patterns = append(patterns, filepath.Join(dir, line))
			} else {
				patterns = append(patterns, line)
			}
		}
	}

	color.Green("  📋 Loaded %d .gitignore patterns", len(patterns))
	return ignore.CompileIgnoreLines(patterns...), nil
}

// Helper function to format file sizes
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// Helper function to get file size
func getFileSize(file *os.File) int64 {
	info, err := file.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
} 