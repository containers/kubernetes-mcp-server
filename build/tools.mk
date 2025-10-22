# Tools

# Platform detection
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')
ifeq ($(ARCH),x86_64)
    ARCH = amd64
endif
ifeq ($(ARCH),aarch64)
    ARCH = arm64
endif

KIND = _output/bin/kind
KIND_VERSION = v0.30.0
$(KIND):
	@mkdir -p _output/bin
	GOBIN=$(PWD)/_output/bin go install sigs.k8s.io/kind@$(KIND_VERSION)

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary
