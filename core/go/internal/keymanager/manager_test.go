/*
 * Copyright © 2025 Kaleido, Inc.
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

package keymanager

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signer"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponents struct {
	c  *componentsmocks.AllComponents
	db sqlmock.Sqlmock
}

func newTestSigner(t *testing.T) (context.Context, signer.SigningModule) {
	ctx := context.Background()
	mnemonic := "extra monster happy tone improve slight duck equal sponsor fruit sister rate very bulb reopen mammal venture pull just motion faculty grab tenant kind"
	sm, err := signer.NewSigningModule(ctx, &signerapi.ConfigNoExt{
		KeyDerivation: pldconf.KeyDerivationConfig{
			Type: pldconf.KeyDerivationTypeBIP32,
		},
		KeyStore: pldconf.KeyStoreConfig{
			Type: pldconf.KeyStoreTypeStatic,
			Static: pldconf.StaticKeyStoreConfig{
				Keys: map[string]pldconf.StaticKeyEntryConfig{
					"seed": {
						Encoding: "none",
						Inline:   mnemonic,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	return ctx, sm
}

func newTestKeyManager(t *testing.T, realDB bool, conf *pldconf.KeyManagerConfig, tps []*testPlugin) (context.Context, *keyManager, *mockComponents, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	oldLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.TraceLevel)

	mc := &mockComponents{c: componentsmocks.NewAllComponents(t)}
	componentsmocks := mc.c

	var p persistence.Persistence
	var pDone func()
	var err error
	if realDB {
		p, pDone, err = persistence.NewUnitTestPersistence(ctx, "keymgr")
		require.NoError(t, err)
	} else {
		mp, err := mockpersistence.NewSQLMockProvider()
		require.NoError(t, err)
		p = mp.P
		mc.db = mp.Mock
		pDone = func() {
			require.NoError(t, mp.Mock.ExpectationsWereMet())
		}
	}
	componentsmocks.On("Persistence").Return(p)

	km := NewKeyManager(ctx, conf)

	ir, err := km.PreInit(mc.c)
	require.NoError(t, err)
	assert.NotNil(t, ir)

	err = km.PostInit(mc.c)
	require.NoError(t, err)

	// Register any signing module plugins
	for _, tp := range tps {
		registerTestSigningModule(t, km.(*keyManager), tp)
	}

	err = km.Start()
	require.NoError(t, err)

	return ctx, km.(*keyManager), mc, func() {
		logrus.SetLevel(oldLevel)
		cancelCtx()
		km.Stop()
		pDone()
	}
}

func hdWalletConfig(name, keyPrefix string) *pldconf.WalletConfig {
	return &pldconf.WalletConfig{
		Name:        name,
		KeySelector: keyPrefix,
		Signer: &pldconf.SignerConfig{
			KeyDerivation: pldconf.KeyDerivationConfig{
				Type: pldconf.KeyDerivationTypeBIP32,
			},
			KeyStore: pldconf.KeyStoreConfig{
				Type: pldconf.KeyStoreTypeStatic,
				Static: pldconf.StaticKeyStoreConfig{
					Keys: map[string]pldconf.StaticKeyEntryConfig{
						"seed": {
							Encoding: "hex",
							Inline:   pldtypes.RandHex(32),
						},
					},
				},
			},
		},
	}

}

func staticKeyConfig(name, keyPrefix string, keys ...string) *pldconf.WalletConfig {
	wc := &pldconf.WalletConfig{
		Name:        name,
		KeySelector: keyPrefix,
		Signer: &pldconf.SignerConfig{
			KeyStore: pldconf.KeyStoreConfig{
				Type: pldconf.KeyStoreTypeStatic,
				Static: pldconf.StaticKeyStoreConfig{
					Keys: map[string]pldconf.StaticKeyEntryConfig{},
				},
			},
		},
	}
	for _, keyName := range keys {
		wc.Signer.KeyStore.Static.Keys[keyName] = pldconf.StaticKeyEntryConfig{
			Encoding: "hex",
			Inline:   pldtypes.RandHex(32),
		}
	}
	return wc
}

func newTestDBKeyManagerWithWallets(t *testing.T, wallets ...*pldconf.WalletConfig) (context.Context, *keyManager, *mockComponents, func()) {
	return newTestKeyManager(t, true, &pldconf.KeyManagerConfig{
		Wallets: append([]*pldconf.WalletConfig{}, wallets...),
	}, nil)
}

func TestConfiguredSigningModules(t *testing.T) {
	_, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Plugin: pldconf.PluginConfig{
					Type:    string(pldtypes.LibraryTypeCShared),
					Library: "some/where",
				},
			},
		},
	}, nil)
	defer done()

	assert.Equal(t, map[string]*pldconf.PluginConfig{
		"test1": {
			Type:    string(pldtypes.LibraryTypeCShared),
			Library: "some/where",
		},
	}, km.ConfiguredSigningModules())
}

func TestGetSigningModule(t *testing.T) {
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, csmr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			return &prototk.ConfigureSigningModuleResponse{}, nil
		},
	}

	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
		Wallets: []*pldconf.WalletConfig{{
			Name:             "signingModuleWallet",
			KeySelector:      "",
			SignerType:       pldconf.WalletSignerTypePlugin,
			SignerPluginName: "test1",
		}},
	}, []*testPlugin{tp})
	defer done()

	assert.Len(t, km.walletsOrdered, 1)

	sm, err := km.GetSigningModule(ctx, "test1")
	assert.NoError(t, err)
	assert.NotNil(t, sm)
	assert.NotNil(t, km.walletsOrdered[0].signingModule)
	assert.NotNil(t, km.walletsByName["signingModuleWallet"].signingModule)

	// For compleness add a new wallet that also uses that signing module
	_, err = km.newWallet(ctx, &pldconf.WalletConfig{
		Name:             "signingModuleWalletNew",
		KeySelector:      "",
		SignerType:       pldconf.WalletSignerTypePlugin,
		SignerPluginName: "test1",
	})

	assert.NoError(t, err)
}

func TestSigningModuleRegisteredNotFound(t *testing.T) {
	_, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{},
	}, nil)
	defer done()

	_, err := km.SigningModuleRegistered("unknown", uuid.New(), nil)
	assert.Regexp(t, "PD010515", err)
}

func TestConfigureSigningModuleFail(t *testing.T) {
	_, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
	}, nil)
	defer done()

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, ctr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			return nil, fmt.Errorf("pop")
		},
	}

	registerTestSigningModule(t, km, tp)
	assert.Regexp(t, "pop", *tp.sm.initError.Load())
}

func TestGetSigningModuleNotFound(t *testing.T) {
	ctx, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{},
	}, nil)
	defer done()

	_, err := km.GetSigningModule(ctx, "unknown")
	assert.Regexp(t, "PD010515", err)
}

func TestConfiguredSigningModulesNoWallet(t *testing.T) {
	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, csmr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			return &prototk.ConfigureSigningModuleResponse{}, nil
		},
	}

	_, km, _, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
		Wallets: []*pldconf.WalletConfig{{
			Name:             "signingModuleWallet",
			KeySelector:      "",
			SignerType:       pldconf.WalletSignerTypePlugin,
			SignerPluginName: "test1",
		}},
	}, []*testPlugin{tp})
	defer done()

	// Set to have no wallets
	km.walletsByName = map[string]*wallet{}
	km.walletsOrdered = []*wallet{}

	configuredSigningModules := km.ConfiguredSigningModules()

	assert.Equal(t, 1, len(configuredSigningModules))
	assert.NotNil(t, configuredSigningModules["test1"])

	assert.Equal(t, 0, len(km.walletsByName))
}

func TestE2ESigningHDWalletRealDB(t *testing.T) {
	ctx, km, _, done := newTestDBKeyManagerWithWallets(t,
		hdWalletConfig("hdwallet1", ""),
	)
	defer done()

	// Sub-test one - repeated resolution of a complex tree
	for i := 0; i < 4; i++ {
		// - first run creates
		// - second run validates they don't change with caching
		// - third run checks they don't change with reload
		if i == 2 {
			km.identifierCache.Clear()
			km.verifierByIdentityCache.Clear()
		}
		// - fourth run just clears the verifiers
		if i == 3 {
			km.verifierByIdentityCache.Clear()
		}

		err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			kr := km.KeyResolverForDBTX(dbTX)

			// one key out of the blue
			resolved1, err := kr.ResolveKey(ctx, "bob.keys.blue.42", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved1.Verifier.Algorithm)
			assert.Equal(t, verifiers.ETH_ADDRESS, resolved1.Verifier.Type)
			assert.Equal(t, "m/44'/60'/1'/0/0/0", resolved1.KeyHandle)

			// sign and recover something
			payload := []byte("some data")
			signature, err := km.Sign(ctx, resolved1, signpayloads.OPAQUE_TO_RSV, payload)
			require.NoError(t, err)
			sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
			require.NoError(t, err)
			addr, err := sig.RecoverDirect(payload, 0)
			require.NoError(t, err)
			assert.Equal(t, addr.String(), resolved1.Verifier.Verifier)

			// a root key, after we've already allocated a key under it
			resolved2, err := kr.ResolveKey(ctx, "bob", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved2.Verifier.Algorithm)
			assert.Equal(t, verifiers.ETH_ADDRESS, resolved2.Verifier.Type)
			assert.Equal(t, "m/44'/60'/1'", resolved2.KeyHandle)

			// keys at a nested layer
			for i := 0; i < 10; i++ {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("bob.keys.red.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/1'/0/1/%d", i), resolved.KeyHandle)
			}

			// same keys backwards
			for i := 9; i >= 0; i-- {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("bob.keys.red.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/1'/0/1/%d", i), resolved.KeyHandle)
			}

			// keys under a different root
			for i := 0; i < 10; i++ {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("sally.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/2'/%d", i), resolved.KeyHandle)
			}

			return nil
		})
		require.NoError(t, err)
	}

	// Sub-test two - concurrent resolution with a consistent outcome
	testUUIDs := make([]uuid.UUID, 15)
	for i := 0; i < len(testUUIDs); i++ {
		testUUIDs[i] = uuid.New()
	}
	const threadCount = 10
	results := make([]map[uuid.UUID]string, threadCount)

	// With this slightly more realistic example of use, we do a proper
	// defer style processing like all the code that uses us in anger should
	// do, ensuring we either commit or cancel.
	testResolveOne := func(identifier string) string {
		resolved, err := km.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
		require.NoError(t, err)
		return resolved[0].String()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < threadCount; i++ {
		wg.Add(1)
		results[i] = make(map[uuid.UUID]string)
		go func() {
			defer wg.Done()
			for _, u := range testUUIDs {
				results[i][u] = testResolveOne(fmt.Sprintf("sally.rand.%s", u))
			}
		}()
	}
	wg.Wait()
	reference := results[0]
	for i := 0; i < threadCount; i++ {
		for _, u := range testUUIDs {
			result, found := results[i][u]
			require.True(t, found)
			require.Equal(t, reference[u], result)
			// Check the reverse lookup too
			resolved, err := km.ReverseKeyLookup(ctx, km.p.NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, result)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("sally.rand.%s", u), resolved.Identifier)
		}
	}

	testResolveMulti := func(doResolve func(kr components.KeyResolver)) {
		// DB TX for each UUID to hammer things a little
		err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			doResolve(km.KeyResolverForDBTX(dbTX))
			return nil
		})
		require.NoError(t, err)
	}

	// Now this last one is really hard.
	// We're trying to create parallelism, but have the potential for an A-B B-A deadlock.
	// Requires PostgreSQL to create the conditions for a hang (multiple parallel DB transactions)
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			testResolveMulti(func(kr components.KeyResolver) {
				_, err := kr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		go func() {
			defer wg.Done()
			testResolveMulti(func(kr components.KeyResolver) {
				_, err := kr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		go func() {
			defer wg.Done()
			testResolveMulti(func(lr components.KeyResolver) {
				_, err := lr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = lr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = lr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		wg.Wait()
	}
}

func TestE2ESigningModulePluginRealDB(t *testing.T) {
	// simple in memory signer
	_, signer := newTestSigner(t)

	tp := newTestPlugin(nil)
	tp.Functions = &plugintk.SigningModuleAPIFunctions{
		ConfigureSigningModule: func(ctx context.Context, csmr *prototk.ConfigureSigningModuleRequest) (*prototk.ConfigureSigningModuleResponse, error) {
			return &prototk.ConfigureSigningModuleResponse{}, nil
		},
		ResolveKey: func(ctx context.Context, rkr *prototk.ResolveKeyRequest) (*prototk.ResolveKeyResponse, error) {
			return signer.Resolve(ctx, rkr)
		},
		Sign: func(ctx context.Context, swkr *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
			return signer.Sign(ctx, swkr)
		},
	}

	ctx, km, _, done := newTestKeyManager(t, true, &pldconf.KeyManagerConfig{
		SigningModules: map[string]*pldconf.SigningModuleConfig{
			"test1": {
				Config: map[string]any{"some": "conf"},
			},
		},
		Wallets: []*pldconf.WalletConfig{{
			Name:             "test-plugin-wallet",
			KeySelector:      "",
			SignerType:       pldconf.WalletSignerTypePlugin,
			SignerPluginName: "test1",
		}},
	}, []*testPlugin{tp})
	defer done()

	registerTestSigningModule(t, km, tp)

	// Sub-test one - repeated resolution of a complex tree
	for i := 0; i < 4; i++ {
		// - first run creates
		// - second run validates they don't change with caching
		// - third run checks they don't change with reload
		if i == 2 {
			km.identifierCache.Clear()
			km.verifierByIdentityCache.Clear()
		}
		// - fourth run just clears the verifiers
		if i == 3 {
			km.verifierByIdentityCache.Clear()
		}

		err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			kr := km.KeyResolverForDBTX(dbTX)

			// one key out of the blue
			resolved1, err := kr.ResolveKey(ctx, "bob.keys.blue.42", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved1.Verifier.Algorithm)
			assert.Equal(t, verifiers.ETH_ADDRESS, resolved1.Verifier.Type)
			assert.Equal(t, "m/44'/60'/1'/0/0/0", resolved1.KeyHandle)

			// sign and recover something
			payload := []byte("some data")
			signature, err := km.Sign(ctx, resolved1, signpayloads.OPAQUE_TO_RSV, payload)
			require.NoError(t, err)
			sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
			require.NoError(t, err)
			addr, err := sig.RecoverDirect(payload, 0)
			require.NoError(t, err)
			assert.Equal(t, addr.String(), resolved1.Verifier.Verifier)

			// a root key, after we've already allocated a key under it
			resolved2, err := kr.ResolveKey(ctx, "bob", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			require.NoError(t, err)
			assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved2.Verifier.Algorithm)
			assert.Equal(t, verifiers.ETH_ADDRESS, resolved2.Verifier.Type)
			assert.Equal(t, "m/44'/60'/1'", resolved2.KeyHandle)

			// keys at a nested layer
			for i := 0; i < 10; i++ {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("bob.keys.red.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/1'/0/1/%d", i), resolved.KeyHandle)
			}

			// same keys backwards
			for i := 9; i >= 0; i-- {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("bob.keys.red.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/1'/0/1/%d", i), resolved.KeyHandle)
			}

			// keys under a different root
			for i := 0; i < 10; i++ {
				resolved, err := kr.ResolveKey(ctx, fmt.Sprintf("sally.%d", i), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				assert.Equal(t, algorithms.ECDSA_SECP256K1, resolved.Verifier.Algorithm)
				assert.Equal(t, verifiers.ETH_ADDRESS, resolved.Verifier.Type)
				assert.Equal(t, fmt.Sprintf("m/44'/60'/2'/%d", i), resolved.KeyHandle)
			}

			return nil
		})
		require.NoError(t, err)
	}

	// Sub-test two - concurrent resolution with a consistent outcome
	testUUIDs := make([]uuid.UUID, 15)
	for i := 0; i < len(testUUIDs); i++ {
		testUUIDs[i] = uuid.New()
	}
	const threadCount = 10
	results := make([]map[uuid.UUID]string, threadCount)

	// With this slightly more realistic example of use, we do a proper
	// defer style processing like all the code that uses us in anger should
	// do, ensuring we either commit or cancel.
	testResolveOne := func(identifier string) string {
		resolved, err := km.ResolveEthAddressBatchNewDatabaseTX(ctx, []string{identifier})
		require.NoError(t, err)
		return resolved[0].String()
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < threadCount; i++ {
		wg.Add(1)
		results[i] = make(map[uuid.UUID]string)
		go func() {
			defer wg.Done()
			for _, u := range testUUIDs {
				results[i][u] = testResolveOne(fmt.Sprintf("sally.rand.%s", u))
			}
		}()
	}
	wg.Wait()
	reference := results[0]
	for i := 0; i < threadCount; i++ {
		for _, u := range testUUIDs {
			result, found := results[i][u]
			require.True(t, found)
			require.Equal(t, reference[u], result)
			// Check the reverse lookup too
			resolved, err := km.ReverseKeyLookup(ctx, km.p.NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, result)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("sally.rand.%s", u), resolved.Identifier)
		}
	}

	testResolveMulti := func(doResolve func(kr components.KeyResolver)) {
		// DB TX for each UUID to hammer things a little
		err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
			doResolve(km.KeyResolverForDBTX(dbTX))
			return nil
		})
		require.NoError(t, err)
	}

	// Now this last one is really hard.
	// We're trying to create parallelism, but have the potential for an A-B B-A deadlock.
	// Requires PostgreSQL to create the conditions for a hang (multiple parallel DB transactions)
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			testResolveMulti(func(kr components.KeyResolver) {
				_, err := kr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+1000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		go func() {
			defer wg.Done()
			testResolveMulti(func(kr components.KeyResolver) {
				_, err := kr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = kr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+2000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		go func() {
			defer wg.Done()
			testResolveMulti(func(lr components.KeyResolver) {
				_, err := lr.ResolveKey(ctx, fmt.Sprintf("path.to.C.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = lr.ResolveKey(ctx, fmt.Sprintf("path.to.B.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
				_, err = lr.ResolveKey(ctx, fmt.Sprintf("path.to.A.%d", i+3000), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				require.NoError(t, err)
			})
		}()
		wg.Wait()
	}

}

func TestE2EMixedKeyResolution(t *testing.T) {
	staticKeys := staticKeyConfig("static", `^static\..*$`, "static.key1", "static.key2")

	ctx, km, _, done := newTestDBKeyManagerWithWallets(t,
		staticKeys,
		hdWalletConfig("hdwallet1", ""),
	)
	defer done()

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)
		_, err := kr.ResolveKey(ctx, "static.key3", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		return err
	})
	assert.Regexp(t, "PD020818", err)

	var mappingStaticKey1, mappingStaticKey2 *pldapi.KeyMappingAndVerifier
	err = km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)
		mappingStaticKey1, err = kr.ResolveKey(ctx, "static.key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
		mappingStaticKey2, err = kr.ResolveKey(ctx, "static.key2", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)

	err = km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)

		key1 := secp256k1.KeyPairFromBytes(pldtypes.MustParseHexBytes(staticKeys.Signer.KeyStore.Static.Keys["static.key1"].Inline))
		require.Equal(t, key1.Address.String(), mappingStaticKey1.Verifier.Verifier)

		key2 := secp256k1.KeyPairFromBytes(pldtypes.MustParseHexBytes(staticKeys.Signer.KeyStore.Static.Keys["static.key2"].Inline))
		require.Equal(t, key2.Address.String(), mappingStaticKey2.Verifier.Verifier)

		_, err = kr.ResolveKey(ctx, "anything.at.any.level", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		assert.NoError(t, err)
		return nil
	})
	require.NoError(t, err)

}

func TestE2EMustNotMatchKeyResolution(t *testing.T) {
	staticKeys := staticKeyConfig("static", `^static\..*$`, "static.key1", "static.key2")
	notStaticKeys := hdWalletConfig("hdwallet1", `^static\..*$`)
	notStaticKeys.KeySelectorMustNotMatch = true

	ctx, km, _, done := newTestDBKeyManagerWithWallets(t,
		notStaticKeys,
		staticKeys,
	)
	defer done()

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)
		_, err := kr.ResolveKey(ctx, "static.key3", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		return err
	})
	assert.Regexp(t, "PD020818", err)

	var mappingStaticKey1, mappingStaticKey2 *pldapi.KeyMappingAndVerifier
	err = km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)
		mappingStaticKey1, err = kr.ResolveKey(ctx, "static.key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
		mappingStaticKey2, err = kr.ResolveKey(ctx, "static.key2", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)

	err = km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {
		kr := km.KeyResolverForDBTX(dbTX)

		key1 := secp256k1.KeyPairFromBytes(pldtypes.MustParseHexBytes(staticKeys.Signer.KeyStore.Static.Keys["static.key1"].Inline))
		require.Equal(t, key1.Address.String(), mappingStaticKey1.Verifier.Verifier)

		key2 := secp256k1.KeyPairFromBytes(pldtypes.MustParseHexBytes(staticKeys.Signer.KeyStore.Static.Keys["static.key2"].Inline))
		require.Equal(t, key2.Address.String(), mappingStaticKey2.Verifier.Verifier)

		_, err = kr.ResolveKey(ctx, "anything.at.any.level", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		assert.NoError(t, err)
		return nil
	})
	require.NoError(t, err)

}

func TestStartFailures(t *testing.T) {
	mc := &mockComponents{c: componentsmocks.NewAllComponents(t)}
	db, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	mc.c.On("Persistence").Return(db.P)

	km := NewKeyManager(context.Background(), &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{
			{ /* no name */ },
		},
	})
	_, err = km.PreInit(mc.c)
	require.NoError(t, err)
	err = km.PostInit(mc.c)
	require.NoError(t, err)
	err = km.Start()
	assert.Regexp(t, "PD010508", err) // no name

	km = NewKeyManager(context.Background(), &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{
			hdWalletConfig("duplicated", ""),
			hdWalletConfig("duplicated", ""),
		},
	})
	_, err = km.PreInit(mc.c)
	require.NoError(t, err)
	err = km.PostInit(mc.c)
	require.NoError(t, err)
	err = km.Start()
	assert.Regexp(t, "PD010509", err) // duplicate name
}

func TestSignUnknownWallet(t *testing.T) {
	ctx, km, _, done := newTestDBKeyManagerWithWallets(t)
	defer done()

	_, err := km.Sign(ctx, &pldapi.KeyMappingAndVerifier{KeyMappingWithPath: &pldapi.KeyMappingWithPath{KeyMapping: &pldapi.KeyMapping{
		Wallet: "unknown",
	}}}, signpayloads.OPAQUE_TO_RSV, []byte{})
	assert.Regexp(t, "PD010503", err)
}

type testSigner struct {
	getMinimumKeyLen func(ctx context.Context, algorithm string) (int, error)
	getVerifier      func(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (string, error)
	sign             func(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) ([]byte, error)
}

func (ts *testSigner) GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error) {
	return ts.getMinimumKeyLen(ctx, algorithm)
}

func (ts *testSigner) GetVerifier(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (string, error) {
	return ts.getVerifier(ctx, algorithm, verifierType, privateKey)
}

func (ts *testSigner) Sign(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) ([]byte, error) {
	return ts.sign(ctx, algorithm, payloadType, privateKey, payload)
}

func TestAddInMemorySignerAndSign(t *testing.T) {

	ctx, km, _, done := newTestDBKeyManagerWithWallets(t, hdWalletConfig("wallet1", ""))
	defer done()

	s := &testSigner{
		getVerifier: func(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
			return "custom-thing", nil
		},
	}
	km.AddInMemorySigner("test", s)

	err := km.p.Transaction(ctx, func(ctx context.Context, dbTX persistence.DBTX) error {

		mapping, err := km.KeyResolverForDBTX(dbTX).ResolveKey(ctx, "my-custom-thing", "test:blue", "thingies")
		require.NoError(t, err)
		require.Equal(t, "test:blue", mapping.Verifier.Algorithm)
		require.Equal(t, "thingies", mapping.Verifier.Type)
		require.Equal(t, "custom-thing", mapping.Verifier.Verifier)

		return nil
	})
	require.NoError(t, err)

}

func TestResolveKeyNewDatabaseTXFail(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnError(fmt.Errorf("pop"))

	_, err := km.ResolveKeyNewDatabaseTX(ctx, "key1", algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	assert.Regexp(t, "pop", err)
}

func TestResolveEthAddressNewDatabaseTXFail(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnError(fmt.Errorf("pop"))

	_, err := km.ResolveEthAddressNewDatabaseTX(ctx, "key1")
	assert.Regexp(t, "pop", err)
}

func TestResolveBatchNewDatabaseTXFail(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectBegin()
	mc.db.ExpectQuery("SELECT.*key_paths").WillReturnError(fmt.Errorf("pop"))

	_, err := km.ResolveBatchNewDatabaseTX(ctx, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, []string{"key1"})
	assert.Regexp(t, "pop", err)
}

func TestReverseKeyLookupFail(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnError(fmt.Errorf("pop"))

	_, err := km.ReverseKeyLookup(ctx, mc.c.Persistence().NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, pldtypes.RandAddress().String())
	assert.Regexp(t, "pop", err)
}

func TestReverseKeyLookupNotFound(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, true, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	_, err := km.ReverseKeyLookup(ctx, mc.c.Persistence().NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, pldtypes.RandAddress().String())
	assert.Regexp(t, "PD010511", err)
}

func TestReverseKeyLookupFailMapping(t *testing.T) {
	ctx, km, mc, done := newTestKeyManager(t, false, &pldconf.KeyManagerConfig{
		Wallets: []*pldconf.WalletConfig{hdWalletConfig("hdwallet1", "")},
	}, nil)
	defer done()

	verifier := pldtypes.RandAddress().String()
	mc.db.ExpectQuery("SELECT.*key_verifiers").WillReturnRows(
		sqlmock.NewRows([]string{"algorithm", "type", "verifier", "identifier"}).
			AddRow(algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, verifier, "!!!!! wrong"),
	)

	_, err := km.ReverseKeyLookup(ctx, mc.c.Persistence().NOTX(), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, verifier)
	assert.Regexp(t, "PD010500", err)
}
