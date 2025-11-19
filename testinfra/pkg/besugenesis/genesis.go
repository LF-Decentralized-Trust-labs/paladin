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

package besugenesis

import (
	"math/big"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/rlp"
)

type GenesisJSON struct {
	Config     GenesisConfig             `json:"config"`
	Nonce      ethtypes.HexUint64        `json:"nonce"`
	Timestamp  ethtypes.HexUint64        `json:"timestamp"`
	GasLimit   ethtypes.HexUint64        `json:"gasLimit"`
	Difficulty ethtypes.HexUint64        `json:"difficulty"`
	MixHash    ethtypes.HexBytes0xPrefix `json:"mixHash"`
	Coinbase   *ethtypes.Address0xHex    `json:"coinbase"`
	Alloc      map[string]AllocEntry     `json:"alloc"`
	ExtraData  ethtypes.HexBytes0xPrefix `json:"extraData"`
}

type GenesisConfig struct {
	ChainID     int64       `json:"chainId"`
	LondonBlock int64       `json:"londonBlock"`
	CancunTime  int64       `json:"cancunTime"`
	ZeroBaseFee *bool       `json:"zeroBaseFee"`
	MinGasPrice *big.Int    `json:"minGasPrice,omitempty"`
	QBFT        *QBFTConfig `json:"qbft,omitempty"`
}

type QBFTConfig struct {
	BlockPeriodSeconds      *int `json:"blockperiodseconds"`
	EpochLength             *int `json:"epochlength"`
	RequestTimeoutSeconds   *int `json:"requesttimeoutseconds,omitempty"`
	EmptyBlockPeriodSeconds *int `json:"xemptyblockperiodseconds,omitempty"`
	BlockPeriodMilliseconds *int `json:"xblockperiodmilliseconds,omitempty"`
}

type AllocEntry struct {
	Balance ethtypes.HexInteger `json:"balance"`
}

func BuildQBFTExtraData(validators ...ethtypes.Address0xHex) []byte {
	vanity := make([]byte, 32)
	copy(vanity, ([]byte)("paladin"))
	var rlpValidators rlp.List
	for _, validator := range validators {
		rlpValidators = append(rlpValidators, rlp.WrapAddress(&validator))
	}
	extraDataRLP := rlp.List{
		// 32 bytes Vanity
		rlp.Data(vanity),
		// List<Validators>
		rlpValidators,
		// No Vote
		rlp.List{},
		// Round=Int(0)
		rlp.WrapInt(big.NewInt(0)),
		// 0 Seals
		rlp.List{},
	}
	return extraDataRLP.Encode()
}
