module github.com/kaleido-io/paladin/domains/integration-test

go 1.23.0

toolchain go1.23.10

require (
	github.com/go-resty/resty/v2 v2.14.0
	github.com/hyperledger/firefly-signer v1.1.21
	github.com/kaleido-io/paladin/common/go v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/config v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/core v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/domains/noto v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/domains/zeto v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/sdk/go v0.0.0-00010101000000-000000000000
	github.com/kaleido-io/paladin/toolkit v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.9.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Code-Hex/go-generics-cache v1.5.1 // indirect
	github.com/aidarkhanov/nanoid v1.0.8 // indirect
	github.com/btcsuite/btcd v0.24.2 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/btcsuite/btcd/btcutil v1.1.6 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dchest/blake512 v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/golang-migrate/migrate/v4 v4.17.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hyperledger-labs/zeto/go-sdk v0.0.0-20241004174307-aa3c1fdf0966 // indirect
	github.com/hyperledger/firefly-common v1.5.4 // indirect
	github.com/iden3/go-iden3-crypto v0.0.17 // indirect
	github.com/iden3/go-rapidsnark/prover v0.0.12 // indirect
	github.com/iden3/go-rapidsnark/types v0.0.3 // indirect
	github.com/iden3/go-rapidsnark/witness/v2 v2.0.0 // indirect
	github.com/iden3/go-rapidsnark/witness/wasmer v0.0.0-20240914111027-9588ce2d7e1b // indirect
	github.com/iden3/wasmer-go v0.0.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.6.0 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	github.com/serialx/hashring v0.0.0-20200727003509-22c0c7ab6b1b // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/x-cray/logrus-prefixed-formatter v0.5.2 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.20.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/term v0.30.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241007155032-5fefd90f89a9 // indirect
	google.golang.org/grpc v1.67.1 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/driver/postgres v1.5.9 // indirect
	gorm.io/driver/sqlite v1.5.6 // indirect
	gorm.io/gorm v1.25.12 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace github.com/kaleido-io/paladin/common/go => ../../common/go

replace github.com/kaleido-io/paladin/core => ../../core/go

replace github.com/kaleido-io/paladin/sdk/go => ../../sdk/go

replace github.com/kaleido-io/paladin/toolkit => ../../toolkit/go

replace github.com/kaleido-io/paladin/domains/noto => ../noto

replace github.com/kaleido-io/paladin/domains/zeto => ../zeto

replace github.com/kaleido-io/paladin/config => ../../config
