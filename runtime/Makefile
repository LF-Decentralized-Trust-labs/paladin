# define the default make task at the top
.PHONY: all
all: test build

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


# define lib specific tassk

# build
.PHONY: clean
clean:
		$(VGO) clean
.PHONY: build    
build: clean deps paladin

.PHONY: paladin
paladin: 
		$(VGO) build -o ./paladin -ldflags "-X main.buildDate=`date -u +\"%Y-%m-%dT%H:%M:%SZ\"` -X main.buildVersion=$(BUILD_VERSION)" -tags=prod -tags=prod -v .