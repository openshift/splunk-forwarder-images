# AGENTS.md

## Repository Overview

splunk-forwarder-images builds the Splunk universal forwarder container images. The repository contains a small Go helper plus container build assets under `build/`.

## Build / Test

- Use `make build` to build the default image from `build/Dockerfile`.
- Use `make test` to run the repository test target.
- Use `make vuln-check` to build the image and run the Clair vulnerability check.
- Konflux PipelineRun definitions live in `.tekton/` for pull request and push builds.

## Working Rules

- Keep Go dependency updates minimal and prefer the smallest viable module bump.
- Go module dependency updates are handled by Konflux MintMaker for this Konflux component.
- Container image builds are handled by the boilerplate `osd-container-image` convention and Konflux pipeline definitions.
- Do not bump boilerplate-managed files, Splunk version pins, or container base images unless the task explicitly asks for those changes.
