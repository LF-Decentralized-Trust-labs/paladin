VGO=go
GOBIN := $(shell $(VGO) env GOPATH)/bin
LINT := $(GOBIN)/golangci-lint

CGO_ENABLED=0
GOGC=30

.DELETE_ON_ERROR:
all: build test go-mod-tidy

test-no-coverage-check: deps lint
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