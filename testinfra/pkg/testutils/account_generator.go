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

package testutils

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip39"
)

// TestAccountGenerator generates predictable test accounts using the same BIP32 derivation as Paladin
type TestAccountGenerator struct {
	hdKeyChain *hdkeychain.ExtendedKey
}

// GenerateAccount generates a test account at the specified derivation path
// The path should be a full BIP32 path like "m/44'/60'/0'/0/0"
func (g *TestAccountGenerator) GenerateAccount(derivationPath string) (*TestAccount, error) {
	// Parse the derivation path (e.g., "m/44'/60'/0'/0/0")
	var segments []string
	if derivationPath != "" {
		// Split the path into segments, removing empty strings
		pathSegments := strings.Split(derivationPath, "/")
		segments = make([]string, 0, len(pathSegments))
		for _, segment := range pathSegments {
			if segment != "" {
				segments = append(segments, segment)
			}
		}
	} else {
		// Default path for first account
		segments = []string{"m", "44'", "60'", "0'", "0", "0"}
	}

	// Validate that path starts with "m"
	if len(segments) == 0 || segments[0] != "m" {
		return nil, fmt.Errorf("derivation path must start with 'm', got: %s", derivationPath)
	}

	// Derive the key starting from the master key
	key := g.hdKeyChain
	for _, segment := range segments[1:] {
		derivation, hardened, err := parseDerivationSegment(segment)
		if err != nil {
			return nil, fmt.Errorf("failed to parse derivation segment %s: %w", segment, err)
		}

		if hardened {
			derivation += 0x80000000
		}

		key, err = key.Derive(uint32(derivation))
		if err != nil {
			return nil, fmt.Errorf("failed to derive key at segment %s: %w", segment, err)
		}
	}

	// Get the private key
	ecPrivKey, err := key.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get EC private key: %w", err)
	}

	// Convert to Ethereum private key
	privateKey := ecPrivKey.ToECDSA()

	// Get the Ethereum address
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	return &TestAccount{
		Address:    address,
		PrivateKey: privateKey,
		Path:       derivationPath,
	}, nil
}

// TestAccount represents a test account with its address and private key
type TestAccount struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
	Path       string
}

// GetAddressString returns the address as a hex string
func (a *TestAccount) GetAddressString() string {
	return a.Address.Hex()
}

// parseDerivationSegment parses a BIP32 derivation segment (e.g., "44'" or "0")
func parseDerivationSegment(segment string) (uint32, bool, error) {
	hardened := false
	if segment[len(segment)-1] == '\'' {
		hardened = true
		segment = segment[:len(segment)-1]
	}

	derivation, err := parseUint32(segment)
	if err != nil {
		return 0, false, err
	}

	return derivation, hardened, nil
}

// parseUint32 parses a string to uint32
func parseUint32(s string) (uint32, error) {
	var result uint32
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// GetPrefundedAccountsWithMultipleSeeds returns accounts generated from multiple seeds and derivation paths
func GetPrefundedAccountsWithMultipleSeeds(seeds []string, derivationPaths []string) (map[string]*big.Int, error) {
	prefunded := make(map[string]*big.Int)
	oneEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	funding := new(big.Int).Mul(oneEth, big.NewInt(1000))

	for i, seed := range seeds {
		generator, err := NewTestAccountGeneratorWithSeed(seed)
		if err != nil {
			return nil, fmt.Errorf("failed to create account generator for seed %d: %w", i, err)
		}

		for _, path := range derivationPaths {
			account, err := generator.GenerateAccount(path)
			if err != nil {
				return nil, fmt.Errorf("failed to generate account for seed %d at path %s: %w", i, path, err)
			}

			address := account.GetAddressString()
			if _, exists := prefunded[address]; exists {
				return nil, fmt.Errorf("duplicate address generated: %s (from seed %d, path %s)", address, i, path)
			}

			prefunded[address] = funding
		}
	}

	return prefunded, nil
}

// NewTestAccountGeneratorWithSeed creates a new account generator using a specific seed
func NewTestAccountGeneratorWithSeed(seed string) (*TestAccountGenerator, error) {
	var seedBytes []byte
	var err error

	// Check if seed is hex-encoded (32 bytes = 64 hex characters)
	if len(seed) == 64 {
		// Hex-encoded seed (like component test seed)
		seedBytes, err = hex.DecodeString(seed)
		if err != nil {
			return nil, fmt.Errorf("failed to decode hex seed: %w", err)
		}
	} else {
		// BIP39 mnemonic seed
		seedBytes = bip39.NewSeed(seed, "")
	}

	// Create BIP32 master key
	hdKeyChain, err := hdkeychain.NewMaster(seedBytes, &chaincfg.MainNetParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create BIP32 master key: %w", err)
	}

	return &TestAccountGenerator{
		hdKeyChain: hdKeyChain,
	}, nil
}
