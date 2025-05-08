package scanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"
)

type TrivyScanner struct {
	client *http.Client
}

type VulnerabilityReport struct {
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
	Summary         Summary         `json:"summary"`
}

type Vulnerability struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	Package     string   `json:"package"`
	Version     string   `json:"version"`
	FixedIn     string   `json:"fixed_in,omitempty"`
	URL         string   `json:"url,omitempty"`
}

type Summary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityUnknown  Severity = "UNKNOWN"
)

type ScanReport struct {
	ImageName       string          `json:"image_name"`
	ScanTime        string          `json:"scan_time"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

type ScanStatus struct {
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Message   string `json:"message"`
	ReportURL string `json:"report_url,omitempty"`
}

func NewTrivyScanner() *TrivyScanner {
	return &TrivyScanner{
		client: &http.Client{},
	}
}

func (s *TrivyScanner) ScanImage(machineID, imageName string) (*ScanReport, error) {
	// Run Trivy scan with optimizations
	scanCmd := fmt.Sprintf(
		"trivy image --format json --no-progress --scanners vuln %s > /tmp/trivy-scan.json 2>&1 & echo $!",
		imageName,
	)
	
	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.fly.io/v1/machines/%s/exec", machineID), bytes.NewBufferString(scanCmd))
	if err != nil {
		return nil, fmt.Errorf("failed to create scan request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to start Trivy scan: %w", err)
	}
	defer resp.Body.Close()

	// Get the process ID
	var pid int
	if err := json.NewDecoder(resp.Body).Decode(&pid); err != nil {
		return nil, fmt.Errorf("failed to get scan process ID: %w", err)
	}

	// Poll for scan completion with timeout
	fmt.Println("Starting vulnerability scan...")
	timeout := 10 * time.Minute // Maximum scan time
	startTime := time.Now()
	lastProgress := time.Now()

	for {
		// Check if process is still running
		checkCmd := fmt.Sprintf("ps -p %d > /dev/null && echo 'running' || echo 'completed'", pid)
		req, err = http.NewRequest("POST", fmt.Sprintf("https://api.fly.io/v1/machines/%s/exec", machineID), bytes.NewBufferString(checkCmd))
		if err != nil {
			return nil, fmt.Errorf("failed to check scan status: %w", err)
		}

		resp, err = s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to check scan status: %w", err)
		}

		var status string
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode status: %w", err)
		}
		resp.Body.Close()

		if status == "completed" {
			break
		}

		// Check for timeout
		if time.Since(startTime) > timeout {
			// Kill the process if it's taking too long
			killCmd := fmt.Sprintf("kill -9 %d", pid)
			req, _ = http.NewRequest("POST", fmt.Sprintf("https://api.fly.io/v1/machines/%s/exec", machineID), bytes.NewBufferString(killCmd))
			s.client.Do(req)
			return nil, fmt.Errorf("scan timed out after %v", timeout)
		}

		// Show progress every 5 seconds
		if time.Since(lastProgress) >= 5*time.Second {
			fmt.Print(".")
			lastProgress = time.Now()
		}

		time.Sleep(1 * time.Second)
	}
	fmt.Println("\nScan completed!")

	// Get the scan results
	readCmd := "cat /tmp/trivy-scan.json"
	req, err = http.NewRequest("POST", fmt.Sprintf("https://api.fly.io/v1/machines/%s/exec", machineID), bytes.NewBufferString(readCmd))
	if err != nil {
		return nil, fmt.Errorf("failed to read scan results: %w", err)
	}

	resp, err = s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to read scan results: %w", err)
	}
	defer resp.Body.Close()

	var trivyReport VulnerabilityReport
	if err := json.NewDecoder(resp.Body).Decode(&trivyReport); err != nil {
		return nil, fmt.Errorf("failed to decode scan results: %w", err)
	}

	// Clean up temporary files
	cleanupCmd := "rm -f /tmp/trivy-scan.json"
	req, _ = http.NewRequest("POST", fmt.Sprintf("https://api.fly.io/v1/machines/%s/exec", machineID), bytes.NewBufferString(cleanupCmd))
	s.client.Do(req)

	// Convert Trivy report to our ScanReport format
	report := &ScanReport{
		ImageName:       imageName,
		ScanTime:        time.Now().Format(time.RFC3339),
		Vulnerabilities: trivyReport.Vulnerabilities,
	}

	return report, nil
}

func (r *ScanReport) FilterBySeverity(minSeverity Severity) *ScanReport {
	filtered := &ScanReport{
		ImageName:       r.ImageName,
		ScanTime:        r.ScanTime,
		Vulnerabilities: make([]Vulnerability, 0),
	}

	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
		SeverityUnknown:  0,
	}

	minSeverityLevel := severityOrder[minSeverity]

	for _, vuln := range r.Vulnerabilities {
		if severityOrder[vuln.Severity] >= minSeverityLevel {
			filtered.Vulnerabilities = append(filtered.Vulnerabilities, vuln)
		}
	}

	return filtered
}

func (r *ScanReport) GenerateTable() string {
	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)

	fmt.Fprintln(w, "SEVERITY\tPACKAGE\tVERSION\tFIXED IN\tTITLE")
	fmt.Fprintln(w, "--------\t-------\t-------\t--------\t-----")

	for _, vuln := range r.Vulnerabilities {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			vuln.Severity,
			vuln.Package,
			vuln.Version,
			vuln.FixedIn,
			vuln.Title,
		)
	}

	w.Flush()
	return sb.String()
}

func (r *ScanReport) GenerateHTML() string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
    <title>Docktor Scan Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .critical { background-color: #ffebee; }
        .high { background-color: #fff3e0; }
        .medium { background-color: #fff8e1; }
        .low { background-color: #f1f8e9; }
        .unknown { background-color: #f5f5f5; }
    </style>
</head>
<body>
    <h1>Docktor Scan Report</h1>
    <p>Image: ` + r.ImageName + `</p>
    <p>Scan Time: ` + r.ScanTime + `</p>
    <table>
        <tr>
            <th>Severity</th>
            <th>Package</th>
            <th>Version</th>
            <th>Fixed In</th>
            <th>Title</th>
            <th>Description</th>
        </tr>`)

	for _, vuln := range r.Vulnerabilities {
		severityClass := strings.ToLower(string(vuln.Severity))
		sb.WriteString(fmt.Sprintf(`
        <tr class="%s">
            <td>%s</td>
            <td>%s</td>
            <td>%s</td>
            <td>%s</td>
            <td>%s</td>
            <td>%s</td>
        </tr>`,
			severityClass,
			vuln.Severity,
			vuln.Package,
			vuln.Version,
			vuln.FixedIn,
			vuln.Title,
			vuln.Description,
		))
	}

	sb.WriteString(`
    </table>
</body>
</html>`)

	return sb.String()
}

func (r *ScanReport) PrintSummary() {
	severityCount := make(map[Severity]int)
	for _, vuln := range r.Vulnerabilities {
		severityCount[vuln.Severity]++
	}

	fmt.Println("\nScan Summary:")
	fmt.Printf("Total vulnerabilities: %d\n", len(r.Vulnerabilities))
	fmt.Printf("Critical: %d\n", severityCount[SeverityCritical])
	fmt.Printf("High: %d\n", severityCount[SeverityHigh])
	fmt.Printf("Medium: %d\n", severityCount[SeverityMedium])
	fmt.Printf("Low: %d\n", severityCount[SeverityLow])
	fmt.Printf("Unknown: %d\n", severityCount[SeverityUnknown])
} 