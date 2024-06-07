PROJ=demo
REMOTE_INSTALL_PROJ=$(PROJ)
IMAGE_ACQUIRER=quay.io/rhsgsa/image-acquirer
IMAGE_ACQUIRER_VERSION=0.2
FRONTEND_IMAGE=quay.io/rhsgsa/threat-frontend
FRONTEND_VERSION=1.92
MOCK_LLM_IMAGE=ghcr.io/kwkoo/mock-llm
BUILDERNAME=multiarch-builder
MODEL_NAME=20240606-nano.pt
MODEL_URL=https://github.com/rhsgsa/yolo-toy-gun/raw/main/weights/$(MODEL_NAME)

BASE:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: configure-infra
configure-infra: configure-user-workload-monitoring deploy-nvidia deploy-kserve-dependencies deploy-oai deploy-minio upload-model
	@echo "done"

# deploys all components to a single OpenShift cluster
.PHONY: deploy
deploy: ensure-logged-in
	oc new-project $(PROJ) 2>/dev/null; \
	if [ $$? -eq 0 ]; then sleep 3; fi
	if [ `oc get limitrange -n $(PROJ) --no-headers 2>/dev/null | wc -l` -gt 0 ]; then \
	  oc delete -n $(PROJ) `oc get limitrange -n $(PROJ) -o name`; \
	fi
	oc apply -n $(PROJ) -k $(BASE)/yaml/overlays/all-in-one/

.PHONY: ensure-logged-in
ensure-logged-in:
	oc whoami
	@echo 'user is logged in'

.PHONY: image-acquirer
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
	  -t $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-amd64 \
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
	  -t $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-arm64 \
	  $(BASE)/image-acquirer
	docker manifest create \
	  $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION) \
	  --amend $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-amd64 \
	  --amend $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-arm64
	docker manifest push --purge $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)
	docker manifest create \
	  $(IMAGE_ACQUIRER):latest \
	  --amend $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-amd64 \
	  --amend $(IMAGE_ACQUIRER):$(IMAGE_ACQUIRER_VERSION)-arm64
	docker manifest push --purge $(IMAGE_ACQUIRER):latest
	@#docker build \
	@#  --rm \
	@#  -t $(IMAGE_ACQUIRER) \
	@#  --build-arg MODEL_NAME=$(MODEL_NAME) \
	@#  --build-arg MODEL_URL=$(MODEL_URL) \
	@#  $(BASE)/image-acquirer

.PHONY: image-frontend
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

.PHONY: image-mock-llm
image-mock-llm: buildx-builder
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/amd64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/amd64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/amd64 \
	  --rm \
	  -t $(MOCK_LLM_IMAGE):amd64 \
	  $(BASE)/mock-llm
	docker buildx build \
	  --push \
	  --provenance false \
	  --sbom false \
	  --platform=linux/arm64 \
	  --cache-to type=local,dest=$(BASE)/docker-cache/arm64,mode=max \
	  --cache-from type=local,src=$(BASE)/docker-cache/arm64 \
	  --rm \
	  -t $(MOCK_LLM_IMAGE):arm64 \
	  $(BASE)/mock-llm
	docker manifest create \
	  $(MOCK_LLM_IMAGE):latest \
	  --amend $(MOCK_LLM_IMAGE):amd64 \
	  --amend $(MOCK_LLM_IMAGE):arm64
	docker manifest push --purge $(MOCK_LLM_IMAGE):latest
	@#docker build --rm -t $(MOCK_LLM_IMAGE) $(BASE)/mock-llm

.PHONY: buildx-builder
buildx-builder:
	-mkdir -p $(BASE)/docker-cache/amd64 $(BASE)/docker-cache/arm64 2>/dev/null
	docker buildx use $(BUILDERNAME) || docker buildx create --name $(BUILDERNAME) --use --buildkitd-flags '--oci-worker-gc-keepstorage 50000'

.PHONY: deploy-nfd
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

.PHONY: deploy-nvidia
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


.PHONY: deploy-kserve-dependencies
deploy-kserve-dependencies:
	@echo "deploying OpenShift Serverless..."
	oc apply -f $(BASE)/yaml/operators/serverless-operator.yaml
	@/bin/echo -n 'waiting for KnativeServing CRD...'
	@until oc get crd knativeservings.operator.knative.dev >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'
	@echo "deploying OpenShift Service Mesh operator..."
	oc apply -f $(BASE)/yaml/operators/service-mesh-operator.yaml
	@/bin/echo -n 'waiting for ServiceMeshControlPlane CRD...'
	@until oc get crd servicemeshcontrolplanes.maistra.io >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'


.PHONY: deploy-oai
deploy-oai:
	@echo "deploying OpenShift AI operator..."
	oc apply -f $(BASE)/yaml/operators/openshift-ai-operator.yaml
	@/bin/echo -n 'waiting for DataScienceCluster CRD...'
	@until oc get crd datascienceclusters.datasciencecluster.opendatahub.io >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo 'done'
	oc apply -f $(BASE)/yaml/operators/datasciencecluster.yaml
	@/bin/echo -n "waiting for inferenceservice-config ConfigMap to appear..."
	@until oc get -n redhat-ods-applications cm/inferenceservice-config >/dev/null 2>/dev/null; do \
	  /bin/echo -n "."; \
	  sleep 5; \
	done
	@echo "done"
	@echo "increasing storage initializer memory limit..."
	# modify storageInitializer memory limit - without this, there is a chance
	# that the storageInitializer initContainer will be OOMKilled
	rm -f /tmp/storageInitializer
	oc extract -n redhat-ods-applications cm/inferenceservice-config --to=/tmp --keys=storageInitializer
	cat /tmp/storageInitializer | sed 's/"memoryLimit": .*/"memoryLimit": "4Gi",/' > /tmp/storageInitializer.new
	oc set data -n redhat-ods-applications cm/inferenceservice-config --from-file=storageInitializer=/tmp/storageInitializer.new
	rm -f /tmp/storageInitializer /tmp/storageInitializer.new
	@/bin/echo -n "waiting for ServiceMeshControlPlane to appear..."
	@until oc get -n istio-system smcp/data-science-smcp >/dev/null 2>/dev/null; do \
	  /bin/echo -n "."; \
	  sleep 5; \
	done
	@echo "done"


.PHONY: deploy-minio
deploy-minio:
	@echo "deploying minio..."
	oc create ns $(PROJ) || echo "$(PROJ) namespace exists"
	oc apply -n $(PROJ) -f $(BASE)/yaml/minio.yaml
	@/bin/echo -n "waiting for minio routes..."
	@until oc get -n $(PROJ) route/minio >/dev/null 2>/dev/null && oc get -n $(PROJ) route/minio-console >/dev/null 2>/dev/null; do \
	  /bin/echo -n '.'; \
	  sleep 5; \
	done
	@echo "done"
	oc set env \
	  -n $(PROJ) \
	  sts/minio \
	  MINIO_SERVER_URL="http://`oc get -n $(PROJ) route/minio -o jsonpath='{.spec.host}'`" \
	  MINIO_BROWSER_REDIRECT_URL="http://`oc get -n $(PROJ) route/minio-console -o jsonpath='{.spec.host}'`"


.PHONY: upload-model
upload-model:
	@echo "removing any previous jobs..."
	-oc delete -n $(PROJ) -k $(BASE)/yaml/base/s3-job/
	@/bin/echo -n "waiting for job to go away..."
	@while [ `oc get -n $(PROJ) --no-headers job/setup-s3 2>/dev/null | wc -l` -gt 0 ]; do \
	  /bin/echo -n "."; \
	done
	@echo "done"
	@echo "creating job to upload model to S3..."
	oc apply -n $(PROJ) -k $(BASE)/yaml/base/s3-job/
	@/bin/echo -n "waiting for pod to show up..."
	@while [ `oc get -n $(PROJ) po -l job=setup-s3 --no-headers 2>/dev/null | wc -l` -lt 1 ]; do \
	  /bin/echo -n "."; \
	  sleep 5; \
	done
	@echo "done"
	@/bin/echo "waiting for pod to be ready..."
	oc wait -n $(PROJ) `oc get -n $(PROJ) po -o name -l job=setup-s3` --for=condition=Ready --timeout=300s
	oc logs -n $(PROJ) -f job/setup-s3
	oc delete -n $(PROJ) -k $(BASE)/yaml/base/s3-job/

.PHONY: deploy-llm
deploy-llm:
	oc create ns $(PROJ) || echo "$(PROJ) namespace exists"
	@echo "deploying inference service..."
	# inference service
	#
	@AWS_ACCESS_KEY_ID="`oc extract secret/minio -n $(PROJ) --to=- --keys=MINIO_ROOT_USER 2>/dev/null`" \
	&& \
	AWS_SECRET_ACCESS_KEY="`oc extract secret/minio -n $(PROJ) --to=- --keys=MINIO_ROOT_PASSWORD 2>/dev/null`" \
	&& \
	echo "AWS_ACCESS_KEY_ID=$$AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY=$$AWS_SECRET_ACCESS_KEY" \
	&& \
	oc kustomize $(BASE)/yaml/base/inferenceservice/ \
	| \
	sed \
	  -e "s/AWS_ACCESS_KEY_ID: .*/AWS_ACCESS_KEY_ID: $$AWS_ACCESS_KEY_ID/" \
	  -e "s/AWS_SECRET_ACCESS_KEY: .*/AWS_SECRET_ACCESS_KEY: $$AWS_SECRET_ACCESS_KEY/" \
	| \
	oc apply -n $(PROJ) -f -
	@/bin/echo -n "waiting for inferenceservice to appear..."
	@until oc get -n $(PROJ) inferenceservice/llm >/dev/null 2>/dev/null; do \
	  /bin/echo -n "."; \
	  sleep 5; \
	done
	@echo "done"

.PHONY: clean-llm
clean-llm:
	oc delete -n $(PROJ) -k $(BASE)/yaml/base/inferenceservice/ || exit 0

.PHONY: configure-user-workload-monitoring
configure-user-workload-monitoring:
	if [ `oc get -n openshift-monitoring cm/cluster-monitoring-config --no-headers 2>/dev/null | wc -l` -lt 1 ]; then \
	  echo 'enableUserWorkload: true' > /tmp/config.yaml; \
	  oc create -n openshift-monitoring cm cluster-monitoring-config --from-file=/tmp/config.yaml; \
	  rm -f /tmp/config.yaml; \
	fi

.PHONY: minio-console
minio-console:
	@echo "http://`oc get -n $(PROJ) route/minio-console -o jsonpath='{.spec.host}'`"

.PHONY: clean-minio
clean-minio:
	oc delete -n $(PROJ) -f $(BASE)/yaml/minio.yaml
	oc delete -n $(PROJ) pvc -l app.kubernetes.io/instance=minio,app.kubernetes.io/name=minio

.PHONY: remote-install
remote-install: clean-remote-install
	oc new-project $(REMOTE_INSTALL_PROJ) || echo "$(REMOTE_INSTALL_PROJ) exists"
	oc create -n $(REMOTE_INSTALL_PROJ) sa remote-installer
	oc adm policy add-cluster-role-to-user -n $(REMOTE_INSTALL_PROJ) cluster-admin -z remote-installer
	oc apply -f $(BASE)/yaml/remote-installer/remote-installer.yaml
	@/bin/echo -n "waiting for job to appear..."
	@until oc get -n $(REMOTE_INSTALL_PROJ) job/remote-installer 2>/dev/null >/dev/null; do \
	  /bin/echo -n "."; \
	  sleep 10; \
	done
	@echo "done"
	oc wait -n $(REMOTE_INSTALL_PROJ) --for condition=ready po -l job-name=remote-installer
	oc logs -n $(REMOTE_INSTALL_PROJ) -f job/remote-installer

.PHONY: clean-remote-install
clean-remote-install:
	-oc delete -n $(REMOTE_INSTALL_PROJ) job/remote-installer
	-oc delete -n $(REMOTE_INSTALL_PROJ) sa/remote-installer
	-for s in `oc get clusterrolebinding -o jsonpath='{.items[?(@.subjects[0].name == "remote-installer")].metadata.name}'`; do \
	  oc delete clusterrolebinding $$s; \
	done