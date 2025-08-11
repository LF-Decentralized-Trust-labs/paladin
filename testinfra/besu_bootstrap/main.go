/*
 * Copyright © 2024 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/testinfra/pkg/besugenesis"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
)

func main() {
	// NOTE: To get the right permissions, this needs to run inside docker against the volume of Besu

	// Validate dir is ok
	if len(os.Args) < 2 {
		exitErrorf("missing directory")
	}
	dir := os.Args[1]
	dataDir := path.Join(dir, "data")
	keyFile := path.Join(dir, "key")
	keyPubFile := path.Join(dir, "key.pub")
	genesisFile := path.Join(dir, "genesis.json")

	if !fileExists(dir) {
		mkdir(dir)
	}
	if !fileExists(dataDir) {
		mkdir(dataDir)
	}

	// Check not already initialized
	if fileExists(keyFile) || fileExists(keyPubFile) || fileExists(genesisFile) {
		fmt.Println("already initialized")
		osExit(0) // this is ok - nothing to do
	}

	// Generate the key
	kp, _ := secp256k1.GenerateSecp256k1KeyPair()
	writeFileStr(keyFile, (ethtypes.HexBytes0xPrefix)(kp.PrivateKeyBytes()))
	writeFileStr(keyPubFile, (ethtypes.HexBytes0xPrefix)(kp.PublicKeyBytes()))

	// Write the genesis
	oneEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	genesis := &besugenesis.GenesisJSON{
		Config: besugenesis.GenesisConfig{
			ChainID:     1337,
			CancunTime:  0,
			ZeroBaseFee: ptrTo(true),
			QBFT: &besugenesis.QBFTConfig{
				BlockPeriodSeconds:      ptrTo(1), // this is overwritten by the BlockPeriodMilliseconds
				EpochLength:             ptrTo(30000),
				RequestTimeoutSeconds:   ptrTo(10),
				EmptyBlockPeriodSeconds: ptrTo(10),
				BlockPeriodMilliseconds: ptrTo(200),
			},
		},
		Nonce:      0,
		Timestamp:  ethtypes.HexUint64(time.Now().Unix()),
		GasLimit:   30 * 1000000,
		Difficulty: 1,
		MixHash:    randBytes(32),
		Coinbase:   ethtypes.MustNewAddress("0x0000000000000000000000000000000000000000"),
		Alloc: map[string]besugenesis.AllocEntry{
			kp.Address.String(): {
				Balance: *ethtypes.NewHexInteger(
					new(big.Int).Mul(oneEth, big.NewInt(1000000000)),
				),
			},
		},
		ExtraData: besugenesis.BuildQBFTExtraData(kp.Address),
	}
	writeFileJSON(genesisFile, &genesis)

}

var osExit = os.Exit

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	osExit(1)
}

func mkdir(dir string) {
	err := os.Mkdir(dir, 0777)
	if err != nil {
		exitErrorf("failed to make dir %q: %s", dir, err)
	}
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func writeFileStr(filename string, stringable fmt.Stringer) {
	writeFile(filename, ([]byte)(stringable.String()))
}

func writeFileJSON(filename string, jsonable any) {
	b, err := json.MarshalIndent(jsonable, "", "  ")
	if err != nil {
		exitErrorf("failed to marshal %T: %s", jsonable, err)
	}
	writeFile(filename, b)
}

func writeFile(filename string, data []byte) {
	err := os.WriteFile(filename, data, 0666)
	if err != nil {
		exitErrorf("failed to write file %q: %s", filename, err)
	}
}

func randBytes(len int) []byte {
	b := make([]byte, len)
	_, _ = rand.Read(b)
	return b
}

func ptrTo[T any](v T) *T {
	return &v
}
