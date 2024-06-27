VGO=go
GOBIN := $(shell $(VGO) env GOPATH)/bin
LINT := $(GOBIN)/golangci-lint
MOCKERY := $(GOBIN)/mockery

CGO_ENABLED=0
GOGC=30

.DELETE_ON_ERROR:
all: build test go-mod-tidy

.PHONY: btxmanager
btxmanager:
		$(eval BTXM_PATH := $(shell $(VGO) list -f '{{.Dir}}' github.com/kaleido-io/paladin-blockchain-transaction-manager/pkg/manager))	

.PHONY: mock
mock: btxmanager deps
	${MOCKERY} --case underscore --dir ${BTXM_PATH} --name BlockchainTransactionManager --outpkg btxmMocks --output mocks/btxmMocks

.PHONY: test-no-coverage-check
test-no-coverage-check: deps lint mock
		$(VGO) test ./pkg/...  -cover -coverprofile=coverage.txt -covermode=atomic -timeout=30s ${TEST_FLAGS}
coverage.html:
		$(VGO) tool cover -html=coverage.txt

# Target to enforce 100% coverage
test: test-no-coverage-check
	@echo "Checking test coverage..."
	@total_coverage=`$(VGO) tool cover -func=coverage.txt | grep total | awk '{print $$3}'`; \
	if [ "$$total_coverage" != "100.0%" ]; then \
		echo "ERROR: Coverage is not 100% (current coverage: $$total_coverage)"; \
		exit 1; \
	else \
		echo "Coverage is 100%"; \
	fi
	@rm -f coverage.txt


coverage: test coverage.html
lint: 
		$(VGO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
		GOGC=20 $(LINT) run -v --timeout 5m

go-mod-tidy: .ALWAYS
		$(VGO) mod tidy
build: test
.ALWAYS: ;
clean:
		$(VGO) clean
deps:
		$(VGO) get ./pkg/...
		$(VGO) get -t ./pkg/...
		$(VGO) install github.com/vektra/mockery/v2@v2.40.2
