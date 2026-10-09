# AGENTS.md

## Repository Overview

splunk-forwarder-images builds the Splunk Universal Forwarder container image consumed by `splunk-forwarder-operator`. The operator deploys this image as Splunk forwarder pods/DaemonSets; the forwarder image is not baked into the operator image.

## Key Files

- `runner.go` contains the small Go helper that becomes `/runner` in the image.
- `build/Dockerfile` builds the runtime image, installs the Splunk forwarder RPM, and sets `CMD ["/runner"]`.
- `.splunk-version` and `.splunk-version-hash` pin the Splunk Universal Forwarder RPM version and download hash.
- `variables.mk` derives `IMAGE_TAG` as `<splunk-version>-<splunk-hash>-<git-sha>`.
- `.tekton/` contains Konflux PipelineRun definitions for pull request and push builds.
- `boilerplate/generated-includes.mk` pulls in the `osd-container-image` boilerplate convention.

## Image Flow

- This repo builds the Splunk forwarder image consumed by `splunk-forwarder-operator`.
- Konflux builds component images for this repo, and promoted release images are consumed by the operator.
- The operator references the promoted image and digest in its OLM templates and SplunkForwarder CRs.
- The forwarder image is deployed by the operator as workload pods; it is not baked into the operator image.

## Build / Test

- Use `make build` to build the default image from `build/Dockerfile`.
- Use `make test` to run the repository test target; this currently depends on `make vuln-check`.
- Use `make vuln-check` to build the image and run the Clair vulnerability check.
- Use `make build-push` only when testing the app-sre push path and after setting `QUAY_USER` and `QUAY_TOKEN`.
- For a quick Go-only sanity check, use `go test ./...`.

## Dependency Management

- Go module dependency updates are handled by Konflux MintMaker for this Konflux component.
- Do not add overlapping Dependabot or Renovate configuration unless the repo is explicitly moved away from MintMaker.
- Keep Go dependency updates minimal and prefer the smallest viable module bump.

## Working Rules

- Do not bump `.splunk-version`, `.splunk-version-hash`, container base images, or boilerplate-managed files unless the task explicitly asks for those changes.
- Preserve the Konflux `.tekton/` PipelineRuns when making unrelated changes.
- When changing the Splunk version pins, verify that the RPM URL in `build/Dockerfile` resolves and coordinate the corresponding operator image-digest update.
- Treat `build/Dockerfile.olm-registry` as legacy/operator-registry support; avoid changing it unless the task is specifically about registry image behavior.
