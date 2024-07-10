# Copyright © 2024 Kaleido, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
# the License. You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
# an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
# specific language governing permissions and limitations under the License.
#
# SPDX-License-Identifier: Apache-2.0


# define the default make task at the top
.PHONY: all
all: test build

# Mockery setup to override the default mock in common task below
.PHONY: setbtxmanagerpath settxmanagerpath
setbtxmanagerpath:
	$(eval BTXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager))
settxmanagerpath:
	$(eval TXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-transaction-manager/pkg/manager))

# include the common tasks
CREATE_TEST_DB := 0
include ../Makefile.common

# register mocks
$(eval $(call makemock, $$(BTXM_PATH), BlockchainTransactionManager, btxmMocks, setbtxmanagerpath))
$(eval $(call makemock, $$(TXM_PATH), PaladinTransactionManager, txmMocks, settxmanagerpath))

# build
.PHONY: clean
clean:
	$(VGO) clean
.PHONY: build    
build: clean deps paladin

.PHONY: paladin
paladin: 
	$(VGO) build -o ./paladin -ldflags "-X main.buildDate=`date -u +\"%Y-%m-%dT%H:%M:%SZ\"` -X main.buildVersion=$(BUILD_VERSION)" -tags=prod -tags=prod -v .