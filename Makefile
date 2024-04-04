PROJ=demo
IMAGE_ACQUIRER=ghcr.io/kwkoo/image-acquirer
FRONTEND_IMAGE=ghcr.io/rhsgsa/threat-frontend
FRONTEND_VERSION=1.9
MOCK_OLLAMA_IMAGE=ghcr.io/kwkoo/mock-ollama
BUILDERNAME=multiarch-builder
MODEL_NAME=NCS_YOLOv8-20Epochs.pt
MODEL_URL=https://github.com/rhsgsa/threat-detection-demo/releases/download/v0.1/$(MODEL_NAME)

BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: deploy image-acquirer image-frontend image-mock-ollama buildx-builder ensure-logged-in deploy-nfd deploy-nvidia

# deploys all components to a single OpenShift cluster
deploy: ensure-logged-in
	oc new-project $(PROJ) 2>/dev/null; \
	if [ $$? -eq 0 ]; then sleep 3; fi
	oc get limitrange -n $(PROJ) >/dev/null 2>/dev/null \
	&& \
	if [ $$? -eq 0 ]; then \
	  oc get limitrange -n $(PROJ) -o name | xargs oc delete -n $(PROJ); \
	fi
	oc apply -n $(PROJ) -k $(BASE)/yaml/overlays/all-in-one/

ensure-logged-in:
	oc whoami
	@echo 'user is logged in'

image-acquirer: buildx-builder
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/amd64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/amd64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/amd64 \
	  --rm \
	  --build-arg MODEL_NAME=$(MODEL_NAME) \
	  --build-arg MODEL_URL=$(MODEL_URL) \
	  -t $(IMAGE_ACQUIRER):amd64 \
	  $(BASE)/image-acquirer
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/arm64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/arm64 \
	  --rm \
	  --build-arg MODEL_NAME=$(MODEL_NAME) \
	  --build-arg MODEL_URL=$(MODEL_URL) \
	  -t $(IMAGE_ACQUIRER):arm64 \
	  $(BASE)/image-acquirer
	docker manifest create \
	  $(IMAGE_ACQUIRER):latest \
	  --amend $(IMAGE_ACQUIRER):amd64 \
	  --amend $(IMAGE_ACQUIRER):arm64
	docker manifest push --purge $(IMAGE_ACQUIRER):latest
	@#docker build \
	@#  --rm \
	@#  -t $(IMAGE_ACQUIRER) \
	@#  --build-arg MODEL_NAME=$(MODEL_NAME) \
	@#  --build-arg MODEL_URL=$(MODEL_URL) \
	@#  $(BASE)/image-acquirer

image-frontend: buildx-builder
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/amd64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/amd64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/amd64 \
	  --rm \
	  --progress plain \
	  -t $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-amd64 \
	  $(BASE)/frontend
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/arm64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/arm64 \
	  --rm \
	  --progress plain \
	  -t $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-arm64 \
	  $(BASE)/frontend
	docker manifest create \
	  $(FRONTEND_IMAGE):$(FRONTEND_VERSION) \
	  --amend $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-amd64 \
	  --amend $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-arm64
	docker manifest push --purge $(FRONTEND_IMAGE):$(FRONTEND_VERSION)
	docker manifest create \
	  $(FRONTEND_IMAGE):latest \
	  --amend $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-amd64 \
	  --amend $(FRONTEND_IMAGE):$(FRONTEND_VERSION)-arm64
	docker manifest push --purge $(FRONTEND_IMAGE):latest
	@#docker build --rm -t $(FRONTEND_IMAGE) $(BASE)/frontend

image-mock-ollama: buildx-builder
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/amd64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/amd64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/amd64 \
	  --rm \
	  -t $(MOCK_OLLAMA_IMAGE):amd64 \
	  $(BASE)/mock-ollama
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/arm64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/arm64 \
	  --rm \
	  -t $(MOCK_OLLAMA_IMAGE):arm64 \
	  $(BASE)/mock-ollama
	docker manifest create \
	  $(MOCK_OLLAMA_IMAGE):latest \
	  --amend $(MOCK_OLLAMA_IMAGE):amd64 \
	  --amend $(MOCK_OLLAMA_IMAGE):arm64
	docker manifest push --purge $(MOCK_OLLAMA_IMAGE):latest
	@#docker build --rm -t $(MOCK_OLLAMA_IMAGE) $(BASE)/mock-ollama

buildx-builder:
	-mkdir -p $(BASE)/docker-cache/amd64 $(BASE)/docker-cache/arm64 2>/dev/null
	docker buildx use $(BUILDERNAME) || docker buildx create --name $(BUILDERNAME) --use --buildkitd-flags '--oci-worker-gc-keepstorage 50000'

deploy-nfd: ensure-logged-in
	@echo "deploying NodeFeatureDiscovery operator..."
	oc apply -f $(BASE)/yaml/operators/nfd-operator.yaml
	@/bin/echo -n 'waiting for NodeFeatureDiscovery CRD...'
	@until oc get crd nodefeaturediscoveries.nfd.openshift.io >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'
	oc apply -f $(BASE)/yaml/operators/nfd-cr.yaml
	@/bin/echo -n 'waiting for nodes to be labelled...'
	@while [ `oc get nodes --no-headers -l 'feature.node.kubernetes.io/pci-10de.present=true' 2>/dev/null | wc -l` -lt 1 ]; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'
	@echo 'NFD operator installed successfully'

deploy-nvidia: deploy-nfd
	@echo "deploying nvidia GPU operator..."
	oc apply -f $(BASE)/yaml/operators/nvidia-operator.yaml
	@/bin/echo -n 'waiting for ClusterPolicy CRD...'
	@until oc get crd clusterpolicies.nvidia.com >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'
	oc apply -f $(BASE)/yaml/operators/cluster-policy.yaml
	@/bin/echo -n 'waiting for nvidia-device-plugin-daemonset...'
	@until oc get -n nvidia-gpu-operator ds/nvidia-device-plugin-daemonset >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo "done"
	@DESIRED="`oc get -n nvidia-gpu-operator ds/nvidia-device-plugin-daemonset -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null`"; \
	if [ "$$DESIRED" -lt 1 ]; then \
	  echo "could not get desired replicas"; \
	  exit 1; \
	fi; \
	echo "desired replicas = $$DESIRED"; \
	/bin/echo -n "waiting for $$DESIRED replicas to be ready..."; \
	while [ "`oc get -n nvidia-gpu-operator ds/nvidia-device-plugin-daemonset -o jsonpath='{.status.numberReady}' 2>/dev/null`" -lt "$$DESIRED" ]; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo "done"
	@echo "checking that worker nodes have access to GPUs..."
	@for po in `oc get po -n nvidia-gpu-operator -o name -l app=nvidia-device-plugin-daemonset`; do \
	  echo "checking $$po"; \
	  oc rsh -n nvidia-gpu-operator $$po nvidia-smi; \
	done
