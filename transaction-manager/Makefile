# define the default make task at the top
.PHONY: all
all: test

# Mockery setup to override the default mock in common task below
MOCK_TARGET_DEFINED := 1
.PHONY: btxmanager
btxmanager:
		$(eval BTXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager))	

.PHONY: mock
mock: deps btxmanager
		${MOCKERY} --case underscore --dir ${BTXM_PATH} --name BlockchainTransactionManager --outpkg btxmMocks --output mocks/btxmMocks


# include the common tasks
include ../Makefile.common
