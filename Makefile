GOPATH := $(shell realpath "$(PWD)/../../")

export GOPATH ...
export GO111MODULE=on

.PHONY: all 
all: clean nfn-operator  ovn4nfvk8s-cni nfn-agent

nfn-operator:
	@go build -o build/bin/nfn-operator ./cmd/nfn-operator

ovn4nfvk8s-cni:
	@go build -o build/bin/ovn4nfvk8s-cni ./cmd/ovn4nfvk8s-cni

nfn-agent:
	@go build -o build/bin/nfn-agent ./cmd/nfn-agent

test:
	@go test -v ./...

clean:
	@rm -f build/bin/ovn4nfvk8s*
	@rm -f build/bin/nfn-operator*
	@rm -f build/bin/nfn-agent*

