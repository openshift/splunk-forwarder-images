# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository builds and maintains container images for Splunk Universal Forwarder. The main components are:

- **Go Runner**: A Go application (`runner.go`) that acts as a wrapper around Splunk, providing health checks, Prometheus metrics, and process management
- **Container Images**: Multi-stage Docker builds that package the Splunk Universal Forwarder with the Go runner
- **CI/CD Pipeline**: Automated builds via OpenShift boilerplate with vulnerability scanning

## Common Commands

### Building and Testing
```bash
# Build the container image
make build-forwarder

# Push the container image
make push-forwarder

# Run vulnerability checks (includes build)
make vuln-check

# Run tests (currently just vuln-check)
make test

# Full build and push pipeline (used in CI)
make build-push
```

### Development
```bash
# Build the Go runner binary
go build -o runner

# Run Go tests
go test ./...

# Update boilerplate
make boilerplate-update
```

## Architecture

### Core Components

1. **runner.go**: Main Go application with these responsibilities:
   - Splunk process lifecycle management (start, restart on failure)
   - Health monitoring via Splunk's REST API (`/services/server/health/splunkd/details`)
   - Prometheus metrics exposition (`/metrics` endpoint)
   - Kubernetes health endpoints (`/livez`, `/healthz`)
   - Log tailing and forwarding

2. **Multi-stage Dockerfile** (`build/Dockerfile`):
   - Builder stage: Compiles Go runner with CGO enabled
   - Runtime stage: UBI 9 minimal with Splunk Universal Forwarder RPM
   - Runs as non-root `splunkfwd` user

3. **Configuration Management**:
   - Splunk version controlled via `.splunk-version` and `.splunk-version-hash` files
   - Image tagging: `${VERSION}-${HASH}-${COMMIT}`
   - Auto-generated admin credentials and API configuration

### Image Registry and Versioning

- Default registry: `quay.io/app-sre/splunk-forwarder`
- Images tagged with Splunk version, hash, and git commit
- Override with `IMAGE_REGISTRY` and `IMAGE_REPOSITORY` variables for local development

### Build Variables

Key variables in `variables.mk`:
- `CONTAINER_ENGINE`: Auto-detects podman or docker
- `IMAGE_TAG`: Computed from Splunk version + git commit
- `DOCKERFILE`: Points to `./build/Dockerfile`
- `QUAY_USER`/`QUAY_TOKEN`: Required for pushing images

## Key Dependencies

- Go 1.24.4 with Prometheus client library
- Splunk Universal Forwarder (version from `.splunk-version`)
- OpenShift boilerplate for CI/CD standards
- UBI 9 minimal base image

## Development Notes

- The Go runner handles Splunk restarts automatically (5-second delay)
- Health checks are exposed on port 8090
- Splunk API configured for local access only (127.0.0.1)
- Log rotation configured for container environments (smaller files, fewer backups)
- Container includes a dummy systemctl wrapper that always fails