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

## Versioning and Tagging
This repository builds container images around the splunk universal forwarder at the version and hash specified in the [.splunk-version](.splunk-version) and [.splunk-version-hash](.splunk-version-hash) files, respectively.
To build around a new version, simply commit a PR updating those files.

## CICD
After a PR merge, an integration job is run by app-sre, triggering a build/push of the image to `quay.io/app-sre/splunk-forwarder`.

To test the app-sre pipeline:
- Create personal repositories and override variables as described [above](#local-buildtest).
- Obtain credentials from your personal repository and set the `QUAY_USER` and `QUAY_TOKEN` variables.
- Run `make build-push`.
