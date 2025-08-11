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
	"encoding/hex"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signpayloads"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
)

type ecdsaSigner struct{}

func (s *ecdsaSigner) Sign(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) ([]byte, error) {
	// We register for all ECDSA algorithms
	curve := strings.TrimPrefix(strings.ToLower(algorithm), algorithms.Prefix_ECDSA+":")
	switch curve {
	case algorithms.Curve_SECP256K1:
		return s.Sign_secp256k1(ctx, algorithm, payloadType, privateKey, payload)
	default:
		return nil, i18n.NewError(ctx, pldmsgs.MsgSigningUnsupportedECDSACurve, curve)
	}
}

func (s *ecdsaSigner) GetVerifier(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
	// We register for all ECDSA algorithms
	curve := strings.TrimPrefix(strings.ToLower(algorithm), algorithms.Prefix_ECDSA+":")
	switch curve {
	case algorithms.Curve_SECP256K1:
		return s.GetVerifier_secp256k1(ctx, algorithm, verifierType, privateKey)
	default:
		return "", i18n.NewError(ctx, pldmsgs.MsgSigningUnsupportedECDSACurve, curve)
	}
}

func (s *ecdsaSigner) Sign_secp256k1(ctx context.Context, algorithm, payloadType string, privateKey, payload []byte) (_ []byte, err error) {
	kp := secp256k1.KeyPairFromBytes(privateKey)
	switch payloadType {
	case signpayloads.OPAQUE_TO_RSV:
		var sig *secp256k1.SignatureData
		if len(payload) == 0 {
			err = i18n.NewError(ctx, pldmsgs.MsgSigningEmptyPayload)
		}
		if err == nil {
			sig, err = kp.SignDirect(payload)
		}
		if err != nil {
			return nil, err
		}
		return sig.CompactRSV(), nil
	default:
		return nil, i18n.NewError(ctx, pldmsgs.MsgSigningUnsupportedPayloadCombination, payloadType, algorithm)
	}
}

func (s *ecdsaSigner) GetVerifier_secp256k1(ctx context.Context, algorithm, verifierType string, privateKey []byte) (string, error) {
	kp := secp256k1.KeyPairFromBytes(privateKey)
	switch verifierType {
	case verifiers.ETH_ADDRESS:
		return ethtypes.Address0xHex(kp.Address).String(), nil
	case verifiers.ETH_ADDRESS_CHECKSUM:
		return ethtypes.AddressWithChecksum(kp.Address).String(), nil
	case verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED_0X:
		return "0x" + hex.EncodeToString(kp.PublicKeyBytes()), nil
	case verifiers.HEX_ECDSA_PUBKEY_UNCOMPRESSED:
		return hex.EncodeToString(kp.PublicKeyBytes()), nil
	default:
		return "", i18n.NewError(ctx, pldmsgs.MsgSigningUnsupportedVerifierCombination, verifierType, algorithm)
	}
}

func (s *ecdsaSigner) GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error) {
	curve := strings.TrimPrefix(strings.ToLower(algorithm), algorithms.Prefix_ECDSA+":")
	switch curve {
	case algorithms.Curve_SECP256K1:
		return 32, nil
	default:
		return -1, i18n.NewError(ctx, pldmsgs.MsgSigningUnsupportedECDSACurve, curve)
	}
}
