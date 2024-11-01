SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build
BUILD_PATH  := $(patsubst %/,%,$(abspath $(dir $(lastword $(MAKEFILE_LIST)))))
PARENT_PATH := $(patsubst %/,%,$(dir $(BUILD_PATH)))
UNITY_PROJ := ${PARENT_PATH}/amp-client-unity
UNITY_PATH := $(shell python3 ${UNITY_PROJ}/amp-utils.py UNITY_PATH "${UNITY_PROJ}")
AMP_UNITY_PATH = ${UNITY_PROJ}/Assets/AMP
PROTOC := ${GOBIN}/protoc

## display this help message
help:
	@echo -e "\033[32m"
	@echo " "
	@echo "  PARENT_PATH:     ${PARENT_PATH}"
	@echo "  BUILD_PATH:      ${BUILD_PATH}"
	@echo "  UNITY_PROJ:      ${UNITY_PROJ}"
	@echo "  UNITY_PATH:      ${UNITY_PATH}"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m  %-32s\033[0m %s\n", $$1, $$2}' | sort


.PHONY: tools generate


## install protobufs tools needed to turn a .proto file into Go and C# files
tools-proto:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install github.com/gogo/protobuf/protoc-gen-gogoslick
	go install github.com/gogo/protobuf/proto
	go get     github.com/gogo/protobuf/jsonpb
	go get     github.com/gogo/protobuf/protoc-gen-gogo
	go get     github.com/gogo/protobuf/gogoproto

