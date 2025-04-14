SHELL := /usr/bin/env bash

QUAY_USER ?=
QUAY_TOKEN ?=
CONTAINER_ENGINE_CONFIG_DIR = .docker

# Accommodate docker or podman
CONTAINER_ENGINE=$(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

SPLUNK_VERSION = $(shell cat .splunk-version)
SPLUNK_HASH = $(shell cat .splunk-version-hash)
CURRENT_COMMIT = $(shell git rev-parse --short=7 HEAD)
IMAGE_TAG = $(SPLUNK_VERSION)-$(SPLUNK_HASH)-$(CURRENT_COMMIT)

IMAGE_REGISTRY ?= quay.io
IMAGE_REPOSITORY ?= app-sre
IMAGE_NAME = splunk-forwarder
IMAGE = $(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(IMAGE_NAME)
IMAGE_URI := $(IMAGE):$(IMAGE_TAG)
DOCKERFILE = ./build/Dockerfile.forwarder
