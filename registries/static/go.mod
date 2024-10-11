module github.com/kaleido-io/paladin/registries/static

go 1.22.5

require (
	github.com/hyperledger/firefly-common v1.4.11
	github.com/kaleido-io/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.9.0
	golang.org/x/crypto v0.27.0
	golang.org/x/text v0.18.0
)

require (
	github.com/aidarkhanov/nanoid v1.0.8 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hyperledger/firefly-signer v1.1.18-0.20240918193554-40e3592a70a1 // indirect
	github.com/kaleido-io/paladin/config v0.0.0-00010101000000-000000000000 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/x-cray/logrus-prefixed-formatter v0.5.2 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/term v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240812133136-8ffd90a71988 // indirect
	google.golang.org/grpc v1.65.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace github.com/kaleido-io/paladin/toolkit => ../../toolkit/go

replace github.com/kaleido-io/paladin/config => ../../config
