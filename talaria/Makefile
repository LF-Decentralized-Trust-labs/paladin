VGO=go
GOBIN := $(shell $(VGO) env GOPATH)/bin
MOCKERY := $(GOBIN)/mockery

proto:
	protoc --go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/plugins/proto/inter-paladin-transport.proto
	protoc --go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		pkg/talaria/proto/plugin-interface.proto



$(MOCKERY):
	$(VGO) install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

.PHONY: mockery
mockery: $(MOCKERY)
	@if [ $(GET_MOCKERY_VERSION) != "$(MOCKERY_VERSION)" ]; then \
		echo "Replacing mockery (wrong version found)" ; \
		$(MAKE) -B mockery; \
	fi

# General dependencies
.PHONY: deps
deps: mockery
	$(VGO) get $(SRC_DIR)
	$(VGO) mod tidy

.PHONY: mocks
define makemock
.PHONY: mocks-$(strip $(1))-$(strip $(2))
mocks: mocks-$(strip $(1))-$(strip $(2))
mocks-$(strip $(1))-$(strip $(2)): mockery $(4)
	$(MOCKERY) --case underscore --dir $(1) --name $(2) --outpkg $(3) --output mocks/$(strip $(3))
endef

$(eval $(call makemock, pkg/talaria, RegistryProvider, registrymocks))
