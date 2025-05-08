# Docktor

A CLI tool for building and scanning Docker images in the cloud using Google Cloud Build. Docktor helps you build and scan your Docker images for vulnerabilities without having to install Docker or Trivy locally.

## Features

- 🚀 Build Docker images in the cloud using Google Cloud Build
- 🔍 Scan images for vulnerabilities using Trivy
- 💰 Free tier usage (120 build-minutes per day)
- 🔒 Secure handling of sensitive information
- 🧹 Automatic cleanup of build artifacts

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

### Building and Scanning an Image

```bash
# Build and scan using default Dockerfile in current directory
docktor scan

# Build and scan using a specific Dockerfile
docktor scan --file path/to/Dockerfile

# Build and scan using a specific build context
docktor scan --context path/to/context
```

### Example

```bash
# Navigate to your project directory
cd my-project

# Initialize Docktor
docktor init

# Build and scan your Docker image
docktor scan
```

## Free Tier Usage

Docktor is optimized to work within Google Cloud Build's free tier limits:
- 120 build-minutes per day
- Builds are automatically configured to use minimal resources
- Build artifacts are automatically cleaned up to save storage

## How It Works

1. **Build Context Upload**: Your build context is compressed and uploaded to Google Cloud Storage
2. **Cloud Build**: The image is built using Google Cloud Build
3. **Vulnerability Scan**: The built image is scanned using Trivy
4. **Results**: Scan results are displayed in your terminal
5. **Cleanup**: Build artifacts are automatically cleaned up

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.