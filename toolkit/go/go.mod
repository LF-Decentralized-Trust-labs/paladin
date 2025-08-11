module github.com/LF-Decentralized-Trust-labs/paladin/toolkit

go 1.24.0

toolchain go1.24.3

require (
	github.com/Code-Hex/go-generics-cache v1.5.1
	github.com/btcsuite/btcd v0.24.2
	github.com/btcsuite/btcd/btcutil v1.1.6
	github.com/go-resty/resty/v2 v2.14.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/hyperledger/firefly-signer v1.1.21
	github.com/LF-Decentralized-Trust-labs/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/LF-Decentralized-Trust-labs/paladin/config v0.0.0-00010101000000-000000000000
	github.com/LF-Decentralized-Trust-labs/paladin/sdk/go v0.0.0-00010101000000-000000000000
	github.com/prometheus/client_golang v1.22.0
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.10.0
	github.com/tyler-smith/go-bip39 v1.1.0
	golang.org/x/text v0.23.0
	google.golang.org/grpc v1.67.1
	google.golang.org/protobuf v1.36.5
	k8s.io/utils v0.0.0-20250321185631-1f6e0b77f77e
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/aidarkhanov/nanoid v1.0.8 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/hyperledger/firefly-common v1.5.4 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/x-cray/logrus-prefixed-formatter v0.5.2 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/term v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241007155032-5fefd90f89a9 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/LF-Decentralized-Trust-labs/paladin/common/go => ../../common/go

replace github.com/LF-Decentralized-Trust-labs/paladin/config => ../../config

replace github.com/LF-Decentralized-Trust-labs/paladin/sdk/go => ../../sdk/go
