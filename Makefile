IMAGE_ACQUIRER=ghcr.io/kwkoo/image-acquirer
IMAGE_ACQUIRER_BASE_IMAGE=nvcr.io/nvidia/cuda:12.3.1-devel-ubi9
BUILDERNAME=multiarch-builder

BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: run image-acquirer

run:
	#docker run \
	#  --name pins \
	#  --rm \
	#  -it \
	#  -p 8080:8080 \
	#  -e VIDEO="rtsp://`ifconfig en0 | grep 'inet ' | awk '{ print $$2 }'`:8554/mystream" \
	#  $(IMAGE)

image-acquirer:
	-mkdir -p $(BASE)/docker-cache
	docker buildx use $(BUILDERNAME) || docker buildx create --name $(BUILDERNAME) --use
	docker buildx build \
	  --push \
	  --platform=linux/amd64,linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache \
	  --rm \
	  --build-arg BASE_IMAGE=$(IMAGE_ACQUIRER_BASE_IMAGE) \
	  -t $(IMAGE_ACQUIRER) \
	  $(BASE)/image-acquirer
	#docker build --rm -t $(IMAGE_ACQUIRER) $(BASE)/image-acquirer

