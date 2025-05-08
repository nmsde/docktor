# Docktor

Docktor is a CLI tool that helps you build, scan, and lint Docker images using Google Cloud Build. It offloads the heavy lifting of building and scanning Docker images to Google Cloud, making it faster and more efficient.

## Features

- 🚀 Offload Docker builds to Google Cloud Build
- 🔍 Scan Docker images for vulnerabilities using Trivy
- 📊 Generate detailed HTML reports of scan results
- 🎯 Lint Dockerfiles using Hadolint for best practices
- 💾 Save scan results locally for future reference
- 🔒 Secure handling of GCP credentials
- 🎨 Beautiful and informative console output
- 📦 Smart context handling with .gitignore support
- 🖼️ Option to pull built images to local Docker daemon

## Prerequisites

1. A Google Cloud Platform account
2. A Google Cloud project with the following APIs enabled:
   - Cloud Build API
   - Cloud Storage API
3. Authentication setup (choose one):
   - A service account with the following roles:
     - Cloud Build Service Account (`roles/cloudbuild.builds.builder`)
     - Storage Object Admin (`roles/storage.objectAdmin`)
     - Service Account User (`roles/iam.serviceAccountUser`)
   - OR set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable

### Setting up a Service Account

1. Go to the Google Cloud Console
2. Navigate to "IAM & Admin" > "Service Accounts"
3. Click "Create Service Account"
4. Enter a name and description for the service account
5. Click "Create and Continue"
6. Grant the following roles:
   - Cloud Build Service Account (`roles/cloudbuild.builds.builder`)
   - Storage Object Admin (`roles/storage.objectAdmin`)
   - Service Account User (`roles/iam.serviceAccountUser`)
7. Click "Continue" and then "Done"
8. Find your new service account in the list and click on it
9. Go to the "Keys" tab
10. Click "Add Key" > "Create new key"
11. Choose JSON format and click "Create"
12. The key file will be downloaded to your computer

## Installation

```bash
# Install using Go
go install github.com/nmsde/docktor@latest

# Or download the latest release from GitHub
# TODO: Add release download instructions
```

## Configuration

1. Initialize Docktor in your project:
```bash
docktor init
```

2. Follow the prompts to enter:
   - Your Google Cloud project ID
   - Your Google Cloud region (default: global)
   - Whether to use a service account key file
   - If yes:
     - Service account email
     - Path to the service account key file

The tool will create a `.docktor.env` file in your project directory with your configuration. This file is automatically added to `.gitignore` to keep your credentials secure.

### Authentication Methods

Docktor supports two authentication methods:

1. **Service Account Key File** (Recommended):
   - Configure during `docktor init`
   - Key file path is stored in `.docktor.env`
   - More secure and portable

2. **Environment Variable**:
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/service-account-key.json"
   ```
   - Useful for CI/CD environments
   - Can be set in your shell or CI/CD configuration

## Usage

### Basic Usage

```bash
# Initialize Docktor with your GCP credentials
docktor init

# Build and scan a Docker image
docktor scan --file path/to/Dockerfile --context .

# Lint a Dockerfile for best practices
docktor lint --file path/to/Dockerfile

# Build an image without scanning
docktor build --file path/to/Dockerfile --context .
```

### Advanced Usage

```bash
# Build and scan with a specific Dockerfile
docktor scan --file path/to/Dockerfile --context .

# Build and scan, then pull the image locally
docktor scan --file path/to/Dockerfile --context . --pull

# Lint a Dockerfile in a specific context
docktor lint 

docktor lint --file path/to/Dockerfile 

# Build an image and pull it locally
docktor build --file path/to/Dockerfile --context . --pull
```

### Command Options

#### Scan Command
- `--file, -f`: Path to the Dockerfile (default: "Dockerfile")
- `--context, -c`: Path to the build context (default: ".")
- `--pull, -p`: Pull the built image to local Docker daemon

#### Lint Command
- `--file, -f`: Path to the Dockerfile (default: "Dockerfile")
- `--context, -c`: Path to the build context (default: ".")

#### Build Command
- `--file, -f`: Path to the Dockerfile (default: "Dockerfile")
- `--context, -c`: Path to the build context (default: ".")
- `--pull, -p`: Pull the built image to local Docker daemon

### Example Workflow

1. Initialize Docktor:
   ```bash
   docktor init
   ```
   - Enter your GCP Project ID
   - Enter your GCP Service Account
   - Enter the path to your GCP Service Key
   - Enter your GCP Region (default: "global")

2. Lint your Dockerfile:
   ```bash
   docktor lint --file Dockerfile
   ```
   This will check your Dockerfile for best practices and common issues.

3. Build and scan your image:
   ```bash
   docktor scan --file Dockerfile --context .
   ```
   This will:
   - Build your Docker image in Google Cloud Build
   - Scan it for vulnerabilities using Trivy
   - Generate an HTML report
   - Optionally pull the image if --pull is specified

4. View the results:
   - The scan results will be saved in the `docktor` directory
   - An HTML report will be generated and opened in your default browser
   - The lint results will be displayed in the console

### Output Files

The tool generates several files in the `docktor` directory:

- `{buildID}-raw.json`: Raw JSON scan results
- `{buildID}-report.html`: Formatted HTML report
- `{buildID}-summary.txt`: Text summary of vulnerabilities

### Build Context

The tool handles the build context intelligently:
- Excludes `node_modules` directories
- Respects `.gitignore` patterns
- Excludes hidden files and directories
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.