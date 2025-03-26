include variables.mk
include boilerplate/generated-includes.mk

.PHONY: build
build:
	$(CONTAINER_ENGINE) build . -f $(DOCKERFILE) -t $(IMAGE_URI)

.PHONY: push
push:
	skopeo copy --dest-creds "$(QUAY_USER):$(QUAY_TOKEN)" "docker-daemon:$(IMAGE_URI)" "docker://$(IMAGE_URI)"

.PHONY: vuln-check
vuln-check: build
	./hack/check-image-against-osd-sre-clair.sh $(IMAGE_URI)

.PHONY: test
test: vuln-check

##################
### Used by CD >>>
.PHONY: build-push
build-push: docker-login
	./hack/app-sre-build-push.sh "$(IMAGE_URI)" "$(DOCKERFILE)"

.PHONY: docker-login
docker-login:
	@test "${QUAY_USER}" != "" && test "${QUAY_TOKEN}" != "" || (echo "QUAY_USER and QUAY_TOKEN must be defined" && exit 1)
	@mkdir -p ${CONTAINER_ENGINE_CONFIG_DIR}
	@${CONTAINER_ENGINE} --config=${CONTAINER_ENGINE_CONFIG_DIR} login -u="${QUAY_USER}" -p="${QUAY_TOKEN}" quay.io

.PHONY: docker-build-push-one
docker-build-push-one:
	@(if [[ -z "${IMAGE_URI}" ]]; then echo "Must specify IMAGE_URI"; exit 1; fi)
	@(if [[ -z "${DOCKERFILE_PATH}" ]]; then echo "Must specify DOCKERFILE_PATH"; exit 1; fi)
	${CONTAINER_ENGINE} build . -f $(DOCKERFILE_PATH) -t $(IMAGE_URI)
	${CONTAINER_ENGINE} --config=${CONTAINER_ENGINE_CONFIG_DIR} push ${IMAGE_URI}
### <<< Used by CD
##################

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
