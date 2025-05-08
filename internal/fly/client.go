package fly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type Client struct {
	apiToken string
	baseURL  string
	client   *http.Client
}

type Machine struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Region string `json:"region"`
	AppID  string `json:"app_id"`
}

type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Region string `json:"region"`
}

func NewClient(apiToken string) (*Client, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("API token is required")
	}

	fmt.Printf("Initializing Fly.io client with token length: %d\n", len(apiToken))

	return &Client{
		apiToken: apiToken,
		baseURL:  "https://api.machines.dev/v1",
		client:   &http.Client{},
	}, nil
}

func (c *Client) CreateApp(orgID, region string) (*App, error) {
	appID := fmt.Sprintf("docktor-%d", os.Getpid())
	
	appPayload := map[string]interface{}{
		"app_name": appID,
		"org_slug": orgID,
	}

	jsonData, err := json.Marshal(appPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal app config: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/apps", c.baseURL), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create app request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create app: %s", string(body))
	}

	var app App
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("failed to decode app response: %w", err)
	}

	return &app, nil
}

func (c *Client) DestroyApp(appID string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/apps/%s", c.baseURL, appID), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to destroy app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to destroy app: %s", string(body))
	}

	return nil
}

func (c *Client) CreateMachine(orgID, region string) (*Machine, error) {
	// First, create an app
	app, err := c.CreateApp(orgID, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}

	// Now create the machine in the app
	machinePayload := map[string]interface{}{
		"name":   fmt.Sprintf("docktor-%d", os.Getpid()),
		"region": region,
		"config": map[string]interface{}{
			"image": "flyio/ubuntu:22.04",
			"services": []map[string]interface{}{
				{
					"protocol": "tcp",
					"ports": []map[string]interface{}{
						{
							"port":     22,
							"handlers": []string{"ssh"},
						},
					},
				},
			},
			"resources": map[string]interface{}{
				"cpu_kind": "shared",
				"cpus":     1,
				"memory_mb": 256, // Free tier limit
			},
			"init": map[string]interface{}{
				"exec": []string{
					// Pre-install Docker to save time
					"apt-get update",
					"apt-get install -y docker.io",
					// Pre-install Trivy to save time
					"curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin",
					// Clean up to save space
					"apt-get clean",
					"rm -rf /var/lib/apt/lists/*",
				},
			},
			"mounts": []map[string]interface{}{}, // No persistent storage
			"env": map[string]string{
				"DOCKER_BUILDKIT": "1", // Enable BuildKit for faster builds
			},
		},
	}

	jsonData, err := json.Marshal(machinePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal machine config: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/apps/%s/machines", c.baseURL, app.Name), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create machine: %s", string(body))
	}

	var machine Machine
	if err := json.NewDecoder(resp.Body).Decode(&machine); err != nil {
		return nil, fmt.Errorf("failed to decode machine response: %w", err)
	}

	// Store the app ID in the machine for cleanup
	machine.AppID = app.Name

	return &machine, nil
}

func (c *Client) UploadProject(machineID, projectPath string) error {
	// Create a new multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Walk through the project directory
	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create a form file
		part, err := writer.CreateFormFile("files", path)
		if err != nil {
			return fmt.Errorf("failed to create form file: %w", err)
		}

		// Open and copy the file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		if _, err := io.Copy(part, file); err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk project directory: %w", err)
	}

	writer.Close()

	// Create the request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/machines/%s/upload", c.baseURL, machineID), body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload project: %s", string(body))
	}

	return nil
}

func (c *Client) BuildImage(machineID string, contextPath string, dockerfilePath string) (string, error) {
	// Create a unique image name
	imageName := fmt.Sprintf("docktor-%s", machineID)

	// Build the Docker image
	cmd := fmt.Sprintf("cd %s && docker build -t %s -f %s .", contextPath, imageName, dockerfilePath)
	if err := c.execCommand(machineID, cmd); err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}

	return imageName, nil
}

func (c *Client) DestroyMachine(machineID string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/machines/%s", c.baseURL, machineID), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to destroy machine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to destroy machine: %s", string(body))
	}

	return nil
}

func (c *Client) execCommand(machineID string, cmd string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/machines/%s/exec", c.baseURL, machineID), bytes.NewBufferString(cmd))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("command execution failed: %s", string(body))
	}

	return nil
} 