PROJ=demo
IMAGE_ACQUIRER=ghcr.io/kwkoo/image-acquirer
IMAGE_ACQUIRER_BASE_IMAGE=nvcr.io/nvidia/cuda:12.3.1-devel-ubi9
FRONTEND_IMAGE=ghcr.io/kwkoo/threat-frontend
MOCK_OLLAMA_IMAGE=ghcr.io/kwkoo/mock-ollama
BUILDERNAME=multiarch-builder
MODEL_URL=https://github.com/rhsgsa/threat-detection-demo/releases/download/v0.1/NCS_YOLOv8-20Epochs.pt

BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: deploy image-acquirer image-frontend ensure-logged-in deploy-nfd deploy-nvidia

# deploys all components to a single OpenShift cluster
deploy: ensure-logged-in
	oc new-project $(PROJ); \
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
	  --build-arg MODEL_URL=$(MODEL_URL) \
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

image-mock-ollama:
	-mkdir -p $(BASE)/docker-cache
	docker buildx use $(BUILDERNAME) || docker buildx create --name $(BUILDERNAME) --use
	docker buildx build \
	  --push \
	  --platform=linux/amd64,linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache \
	  --rm \
	  -t $(MOCK_OLLAMA_IMAGE) \
	  $(BASE)/mock-ollama
	#docker build --rm -t $(MOCK_OLLAMA_IMAGE) $(BASE)/mock-ollama

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
