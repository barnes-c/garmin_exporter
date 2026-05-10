# Copyright 2015 The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Ensure that 'all' is the default target otherwise it will be the first target from Makefile.common.
all::

# Needs to be defined before including Makefile.common to auto-generate targets
DOCKER_ARCHS ?= amd64 arm64 armv7 ppc64le riscv64 s390x

include Makefile.common

DOCKER_IMAGE_NAME       ?= garmin-exporter

STATICCHECK_IGNORE =

# Use CGO for non-Linux builds.
ifeq ($(GOOS), linux)
	PROMU_CONF ?= .promu.yml
else
	ifndef GOOS
		ifeq ($(GOHOSTOS), linux)
			PROMU_CONF ?= .promu.yml
		else
			PROMU_CONF ?= .promu-cgo.yml
		endif
	else
		# Do not use CGO for openbsd/amd64 builds
		ifeq ($(GOOS), openbsd)
			ifeq ($(GOARCH), amd64)
				PROMU_CONF ?= .promu.yml
			else
				PROMU_CONF ?= .promu-cgo.yml
			endif
		else
			PROMU_CONF ?= .promu-cgo.yml
		endif
	endif
endif

PROMU := $(FIRST_GOPATH)/bin/promu --config $(PROMU_CONF)


all:: vet common-all

.PHONY: test
test:
	@echo ">> running tests"
	$(GO) test -short $(test-flags) $(pkgs)

.PHONY: tools
tools:
	@rm ./tools/tools >/dev/null 2>&1 || true
	@$(GO) build -o tools ./tools/...

.PHONY: test-docker
test-docker:
	@echo ">> testing docker image"
	./test_image.sh "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)-linux-amd64:$(DOCKER_IMAGE_TAG)" 9100
