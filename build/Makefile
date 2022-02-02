IMAGE_REGISTRY ?= integratedcloudnative/
BUILD_VERSION ?= master
CNI_VERSION ?= v0.8.5

export NODUS_IMAGE_NAME ?= $(IMAGE_REGISTRY)ovn4nfv-k8s-plugin:$(BUILD_VERSION)

DOCKERARGS?=
ifdef HTTP_PROXY
	DOCKERARGS += --build-arg http_proxy=$(HTTP_PROXY)
endif
ifdef HTTPS_PROXY
	DOCKERARGS += --build-arg https_proxy=$(HTTPS_PROXY)
endif
DOCKERARGS += --build-arg CNI_VERSION=$(CNI_VERSION)

docker-build:
	docker build -t $(NODUS_IMAGE_NAME) $(DOCKERARGS) --network host --no-cache -f ./Dockerfile.ubuntu ../

docker-push:
	docker push $(NODUS_IMAGE_NAME)


