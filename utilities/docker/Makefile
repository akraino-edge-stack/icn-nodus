IMAGE_REGISTRY ?= integratedcloudnative/
BUILD_VERSION ?= master

export OVN_IMAGE_NAME ?= $(IMAGE_REGISTRY)ovn-images:$(BUILD_VERSION)

DOCKERARGS?=
ifdef HTTP_PROXY
	DOCKERARGS += --build-arg http_proxy=$(HTTP_PROXY)
endif
ifdef HTTPS_PROXY
	DOCKERARGS += --build-arg https_proxy=$(HTTPS_PROXY)
endif

docker-build:
	docker build -t $(OVN_IMAGE_NAME) $(DOCKERARGS) --no-cache --network host -f ./debian/Dockerfile .

docker-push:
	docker push $(OVN_IMAGE_NAME)


