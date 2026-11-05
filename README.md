# splunk-forwarder-images
Build and push container images for splunk universal forwarder.

## Local Build/Test
The following `make` variables affect building/pushing:
- `IMAGE_REGISTRY` (default `quay.io`)
- `IMAGE_REPOSITORY` (default `app-sre`)
- `FORWARDER_NAME` (default `splunk-forwarder`)

Images will be tagged and pushed as `${IMAGE_REGISTRY}/${IMAGE_REPOSITORY}/${FORWARDER_NAME}:${VERSION}-${HASH}-${COMMIT}`, where `${VERSION}` and `${HASH}` are gleaned from [.splunk-version](.splunk-version) and [.splunk-version-hash](.splunk-version-hash), respectively, and ${COMMIT} is the 7 char short current commit hash of this repository.
Therefore, for local building and testing, you should create personal image repositories and point to them by overriding at least `IMAGE_REPOSITORY`.

Build images using `make build-forwarder`.

Push images using `make push-forwarder`.

Run vulnerability checks using `make vuln-check`.

## Runtime image

`build/Dockerfile` migrates the existing image in place to the supported
[Red Hat Hummingbird core runtime](https://gitlab.com/redhat/hummingbird/containers).
The runtime and its installer stage are digest-pinned. The installer adds Splunk
and its shell, core utilities, process tools, and libraries to a separate root
filesystem; DNF, RPM, and the download tool stay in the installer stage.
The final image runs `/runner` as `splunkfwd`.

Build with `make build`. The pinned Splunk RPM is x86_64, so on an ARM host use
`DOCKER_DEFAULT_PLATFORM=linux/amd64 make build CONTAINER_ENGINE=docker`.

Konflux continues to build the same Dockerfile and component. Review Hummingbird
base digest updates through MintMaker and the existing build/release gates.
Before promotion, validate forwarding through the operator, health endpoints,
volume permissions, vulnerability scans, Enterprise Contract, and release gates.
FIPS behavior requires separate validation on a FIPS-enabled target cluster.

The legacy `make vuln-check` (also used by `make test`) currently queries
`rpmquery` inside the container and cannot scan this runtime without adaptation.
Use the Konflux image scanning gates to validate the exported image; do not add
package managers to the runtime solely for that helper.

## Versioning and Tagging
This repository builds container images around the splunk universal forwarder at the version and hash specified in the [.splunk-version](.splunk-version) and [.splunk-version-hash](.splunk-version-hash) files, respectively.
To build around a new version, simply commit a PR updating those files.

## CICD
After a PR merge, an integration job is run by app-sre, triggering a build/push of the image to `quay.io/app-sre/splunk-forwarder`.

To test the app-sre pipeline:
- Create personal repositories and override variables as described [above](#local-buildtest).
- Obtain credentials from your personal repository and set the `QUAY_USER` and `QUAY_TOKEN` variables.
- Run `make build-push`.
