PROJ=demo
IMAGE_ACQUIRER=ghcr.io/kwkoo/image-acquirer
IMAGE_ACQUIRER_BASE_IMAGE=nvcr.io/nvidia/cuda:12.3.1-devel-ubi9
FRONTEND_IMAGE=ghcr.io/kwkoo/threat-frontend
BUILDERNAME=multiarch-builder

BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: deploy image-acquirer image-frontend

# deploys all components to a single OpenShift cluster
deploy:
	oc whoami
	oc new-project $(PROJ); \
	if [ $$? -eq 0 ]; then sleep 3; fi
	oc get limitrange -n $(PROJ) >/dev/null 2>/dev/null \
	&& \
	if [ $$? -eq 0 ]; then \
	  oc get limitrange -n $(PROJ) -o name | xargs oc delete -n $(PROJ); \
	fi
	oc apply -n $(PROJ) -k $(BASE)/yaml/overlays/all-in-one/

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

image-frontend:
	-mkdir -p $(BASE)/docker-cache
	docker buildx use $(BUILDERNAME) || docker buildx create --name $(BUILDERNAME) --use
	docker buildx build \
	  --push \
	  --platform=linux/amd64,linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache \
	  --rm \
	  -t $(FRONTEND_IMAGE) \
	  $(BASE)/frontend
	#docker build --rm -t $(FRONTEND_IMAGE) $(BASE)/frontend

