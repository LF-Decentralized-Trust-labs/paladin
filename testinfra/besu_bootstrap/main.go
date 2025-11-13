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
	"flag"
	"fmt"
	"math/big"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/LFDT-Paladin/paladin/testinfra/pkg/besugenesis"
	"github.com/LFDT-Paladin/paladin/testinfra/pkg/testutils"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
)

func main() {
	// NOTE: To get the right permissions, this needs to run inside docker against the volume of Besu

	// Parse command line flags
	var minGasPriceStr string
	var zeroBaseFee bool
	var seeds string
	var derivationPaths string
	flag.StringVar(&minGasPriceStr, "min-gas-price", "0", "Minimum gas price in wei (0 for free gas)")
	flag.BoolVar(&zeroBaseFee, "zero-base-fee", true, "Enable zero base fee (free gas)")
	flag.StringVar(&seeds, "seeds", "", "Comma-separated list of hex-encoded seeds for account generation (optional)")
	flag.StringVar(&derivationPaths, "prefunded-derivation-paths", "", "Comma-separated list of BIP32 derivation paths for prefunded accounts (optional)")
	flag.Parse()

	// Validate dir is ok
	args := flag.Args()
	if len(args) < 1 {
		exitErrorf("missing directory")
	}
	dir := args[0]
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

	// Parse minimum gas price
	var minGasPrice *big.Int
	if minGasPriceStr != "0" {
		gasPrice, err := strconv.ParseInt(minGasPriceStr, 10, 64)
		if err != nil {
			exitErrorf("invalid min-gas-price: %s", err)
		}
		minGasPrice = big.NewInt(gasPrice)
	}

	// Get prefunded test accounts
	var prefundedAccounts map[string]*big.Int
	var err error

	if seeds != "" && derivationPaths != "" {
		// Use custom seeds and derivation paths
		seedList := strings.Split(seeds, ",")
		for i, seed := range seedList {
			seedList[i] = strings.TrimSpace(seed)
		}
		paths := strings.Split(derivationPaths, ",")
		for i, path := range paths {
			paths[i] = strings.TrimSpace(path)
		}
		fmt.Printf("Generating prefunded accounts with %d seeds: %v\n", len(seedList), seedList)
		fmt.Printf("Using derivation paths: %v\n", paths)
		prefundedAccounts, err = testutils.GetPrefundedAccountsWithMultipleSeeds(seedList, paths)
	} else if seeds != "" || derivationPaths != "" {
		// Both parameters must be provided together
		exitErrorf("both --seeds and --prefunded-derivation-paths must be provided together")
	} else {
		// Use default behavior (no prefunded accounts)
		fmt.Println("No prefunded accounts specified")
		prefundedAccounts = make(map[string]*big.Int)
	}

	if err != nil {
		exitErrorf("failed to get prefunded accounts: %s", err)
	}

	// Write the genesis
	oneEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	genesis := &besugenesis.GenesisJSON{
		Config: besugenesis.GenesisConfig{
			ChainID:     1337,
			LondonBlock: 0, // Enable EIP-1559 from genesis
			CancunTime:  0,
			ZeroBaseFee: ptrTo(zeroBaseFee),
			MinGasPrice: minGasPrice,
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
		Alloc:      make(map[string]besugenesis.AllocEntry),
		ExtraData:  besugenesis.BuildQBFTExtraData(kp.Address),
	}

	// Add the validator account (generated key)
	validatorBalance := new(big.Int).Mul(oneEth, big.NewInt(1000000000))
	genesis.Alloc[kp.Address.String()] = besugenesis.AllocEntry{
		Balance: *ethtypes.NewHexInteger(validatorBalance),
	}
	fmt.Printf("Validator account: %s (balance: %s ETH)\n", kp.Address.String(), new(big.Int).Div(validatorBalance, oneEth).String())

	// Add prefunded test accounts
	if len(prefundedAccounts) > 0 {
		fmt.Printf("Prefunded accounts (%d):\n", len(prefundedAccounts))
		for address, balance := range prefundedAccounts {
			genesis.Alloc[address] = besugenesis.AllocEntry{
				Balance: *ethtypes.NewHexInteger(balance),
			}
			fmt.Printf("  %s (balance: %s ETH)\n", address, new(big.Int).Div(balance, oneEth).String())
		}
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
