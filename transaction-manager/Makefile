.PHONY: all
all: test

.PHONY: setbtxmanagerpath
setbtxmanagerpath:
	$(eval BTXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager))

# include the common tasks
USE_TEST_DB := 0
include ../Makefile.common

# register mocks
$(eval $(call makemock, $$(BTXM_PATH), BlockchainTransactionManager, btxmMocks, setbtxmanagerpath))
