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

package signers

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSigner(t *testing.T, privKey []byte) (context.Context, *ecdsaSigner, *secp256k1.KeyPair) {
	ctx := context.Background()

	keypair := secp256k1.KeyPairFromBytes(privKey)

	signerFactory := NewECDSASignerFactory[*signerapi.ConfigNoExt]()
	signer, err := signerFactory.NewSigner(ctx, &signerapi.ConfigNoExt{})
	require.NoError(t, err)

	return ctx, signer.(*ecdsaSigner), keypair
}

func TestErrors(t *testing.T) {

	ctx, signer, _ := newTestSigner(t, pldtypes.RandBytes(32))

	_, err := signer.Sign(ctx, "ecdsa:unknown", "", nil, nil)
	assert.Regexp(t, "PD020822", err)

	_, err = signer.Sign(ctx, "ecdsa:secp256k1", "wrong", nil, nil)
	assert.Regexp(t, "PD020824", err)

	_, err = signer.GetVerifier(ctx, "ecdsa:unknown", "", nil)
	assert.Regexp(t, "PD020822", err)

	_, err = signer.GetMinimumKeyLen(ctx, "ecdsa:unknown")
	assert.Regexp(t, "PD020822", err)

	_, err = signer.GetVerifier(ctx, "ecdsa:secp256k1", "wrong", nil)
	assert.Regexp(t, "PD020823", err)

	_, err = signer.Sign(ctx, "ecdsa:secp256k1", "opaque:rsv", nil, nil)
	assert.Regexp(t, "PD020825", err)

}

func TestECDSASigning_secp256k1(t *testing.T) {
	ctx, signer, kp := newTestSigner(t, pldtypes.RandBytes(32))

	testData := pldtypes.RandBytes(128)

	keyLen, err := signer.GetMinimumKeyLen(ctx, algorithms.ECDSA_SECP256K1)
	require.NoError(t, err)
	assert.Equal(t, 32, keyLen)

	signatureRSV, err := signer.Sign(context.Background(), algorithms.ECDSA_SECP256K1, signpayloads.OPAQUE_TO_RSV, kp.PrivateKeyBytes(), testData)
	require.NoError(t, err)

	sig, err := secp256k1.DecodeCompactRSV(ctx, signatureRSV)
	require.NoError(t, err)
	assert.True(t, sig.V.Int64() == 0 || sig.V.Int64() == 1)

	anyChainID, _ := rand.Int(rand.Reader, big.NewInt(1122334455))
	recovered, err := sig.RecoverDirect(testData,
		anyChainID.Int64(), /* no need for a chain ID as this is opaque signing so 27 or 28 */
	)
	require.NoError(t, err)
	assert.Equal(t, kp.Address, *recovered)

}

func TestECDSAVerifiers_secp256k1(t *testing.T) {
	privKey := ethtypes.MustNewHexBytes0xPrefix(
		"4afef2e65381d2667f0af4347d01d1813f6a7e1824c65c0210a104cc80d3aa15")
	pubKey := "ee2d5c9b18d8301da23217cdea41526ac96e57e2e43ff2d403f1ce90f35044e45cd6741e6ba3ec82882a8a96f57f487a3a0664acd35f03d0529210d2c05e1477"
	addrChecksum := "0x5a8fc778e514420a3FBDceFbb6f1f129A546e96E"

	ctx, signer, kp := newTestSigner(t, privKey)
	require.Equal(t, kp.Address, *ethtypes.MustNewAddress(addrChecksum))

	verifier, err := signer.GetVerifier(ctx, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS, privKey)
	require.NoError(t, err)
	assert.Equal(t, "0x5a8fc778e514420a3fbdcefbb6f1f129a546e96e", verifier)

	verifier, err = signer.GetVerifier(ctx, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS_CHECKSUM, privKey)
	require.NoError(t, err)
	assert.Equal(t, addrChecksum, verifier)

	verifier, err = signer.GetVerifier(ctx, algorithms.ECDSA_SECP256K1, verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED_0X, privKey)
	require.NoError(t, err)
	assert.Equal(t, "0xee2d5c9b18d8301da23217cdea41526ac96e57e2e43ff2d403f1ce90f35044e45cd6741e6ba3ec82882a8a96f57f487a3a0664acd35f03d0529210d2c05e1477", verifier)

	verifier, err = signer.GetVerifier(ctx, algorithms.ECDSA_SECP256K1, verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED, privKey)
	require.NoError(t, err)
	assert.Equal(t, pubKey, verifier)
}
