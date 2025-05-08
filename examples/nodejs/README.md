# Node.js Example App

This is a simple Node.js application used to demonstrate and test Docktor's functionality. It includes:

- Express.js web server
- Basic security middleware (helmet, cors)
- Health check endpoint
- Error handling
- Dockerfile for containerization

## Features

- `/` - Welcome endpoint with timestamp and environment info
- `/health` - Health check endpoint with uptime information
- Error handling middleware
- Production-ready Dockerfile

## Dependencies

- express: Web framework
- cors: Cross-origin resource sharing
- helmet: Security headers
- nodemon: Development server (dev dependency)

## Running Locally

1. Install dependencies:
```bash
npm install
```

2. Start the development server:
```bash
npm run dev
```

3. Start the production server:
```bash
npm start
```

## Building with Docker

```bash
docker build -t docktor-nodejs-example .
docker run -p 3000:3000 docktor-nodejs-example
```

## Testing with Docktor

1. Initialize Docktor configuration:
```bash
docktor init
```

2. Scan the Docker image:
```bash
docktor scan --context .
```

The application will be built and scanned for vulnerabilities using Docktor. 