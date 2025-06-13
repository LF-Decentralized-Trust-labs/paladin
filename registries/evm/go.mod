module github.com/kaleido-io/paladin/registries/evm

go 1.23.0

toolchain go1.23.10

require (
	github.com/google/uuid v1.6.0
	github.com/hyperledger/firefly-signer v1.1.21
	github.com/kaleido-io/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/sdk/go v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.9.0
	golang.org/x/text v0.23.0
)

require (
	github.com/aidarkhanov/nanoid v1.0.8 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/hyperledger/firefly-common v1.5.4 // indirect
	github.com/kaleido-io/paladin/config v0.0.0-00010101000000-000000000000 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/x-cray/logrus-prefixed-formatter v0.5.2 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/term v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241007155032-5fefd90f89a9 // indirect
	google.golang.org/grpc v1.67.1 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/kaleido-io/paladin/common/go => ../../common/go

replace github.com/kaleido-io/paladin/sdk/go => ../../sdk/go

replace github.com/kaleido-io/paladin/toolkit => ../../toolkit/go

replace github.com/kaleido-io/paladin/config => ../../config
