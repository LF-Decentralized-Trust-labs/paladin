.PHONY: all
all: test

.PHONY: btxmanager
btxmanager:
	$(eval BTXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager))

# include the common tasks
include ../Makefile.common

# register mocks
$(eval $(call makemock, $$(BTXM_PATH), BlockchainTransactionManager, btxmMocks, btxmanager))
