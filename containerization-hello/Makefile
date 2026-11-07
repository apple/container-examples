# Copyright © 2025-2026 Apple Inc. and the Containerization project authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# ctr-hello Makefile

SWIFT = /usr/bin/swift

UNAME_M := $(shell uname -m)
KERNEL_ARCH := $(if $(filter $(UNAME_M),aarch64 arm64),arm64,$(UNAME_M))
KERNEL := .state/vmlinux-$(KERNEL_ARCH)
# The kernel that apple/container 1.7.1 installs by default.
KATA_BINARY_PACKAGE := https://github.com/kata-containers/kata-containers/releases/download/4.3.1/kata-static-4.3.1-arm64.tar.zst
KATA_BINARY_SHA256 := 2c5bf7cde334ad5307d482f45cceda496f91c9b6b96f5278c47f4f4eac1bbae6
KATA_KERNEL_PATH := opt/kata/share/kata-containers/vmlinux-6.18.56-208-debug

.PHONY: all build clean clean-state run

all: run

build:
	$(SWIFT) build --configuration release
	codesign --force --sign - --entitlements ctr-hello.entitlements ./.build/release/ctr-hello
	cp ./.build/release/ctr-hello ./ctr-hello

clean:
	$(SWIFT) package clean
	rm -f ./ctr-hello

clean-state:
	rm -rf .state

run: build $(KERNEL)
	./ctr-hello

# Development targets
debug:
	$(SWIFT) build
	codesign --force --sign - --entitlements ctr-hello.entitlements ./.build/debug/ctr-hello

fmt:
	$(SWIFT) format --in-place --recursive Sources/

# Downloads the default kernel into this directory. `make run` does this automatically.
.PHONY: fetch-default-kernel
fetch-default-kernel: $(KERNEL)

$(KERNEL):
	mkdir -p .state
	curl -SsfL -o .state/kata.tar.zst $(KATA_BINARY_PACKAGE)
	echo "$(KATA_BINARY_SHA256)  .state/kata.tar.zst" | shasum -a 256 -c -
	tar -xf .state/kata.tar.zst -C .state ./$(KATA_KERNEL_PATH)
	cp -L .state/$(KATA_KERNEL_PATH) $@
	rm -rf .state/kata.tar.zst .state/opt
