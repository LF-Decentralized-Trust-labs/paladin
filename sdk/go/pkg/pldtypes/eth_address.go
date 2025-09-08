// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldtypes

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

// EthAddress is an SQL serializable version of ethtypes.Address0xHex
type EthAddress [20]byte

var zeroAddress = EthAddress{}

func ParseEthAddress(s string) (*EthAddress, error) {
	a, err := ethtypes.NewAddress(s)
	if err != nil {
		return nil, err
	}
	return (*EthAddress)(a), nil
}

func MustEthAddress(s string) *EthAddress {
	a := ethtypes.MustNewAddress(s)
	return (*EthAddress)(a)
}

func EthAddressBytes(b []byte) *EthAddress {
	var a EthAddress
	copy(a[:], b)
	return &a
}

func RandAddress() *EthAddress {
	return (*EthAddress)(RandBytes(20))
}

func (a *EthAddress) Address0xHex() *ethtypes.Address0xHex {
	return (*ethtypes.Address0xHex)(a)
}

func (a *EthAddress) Checksummed() string {
	return (*ethtypes.AddressWithChecksum)(a).String()
}

func (a *EthAddress) Equals(b *EthAddress) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (a *EthAddress) IsZero() bool {
	return a == nil || *a == zeroAddress
}

func (a EthAddress) String() string {
	return a.Address0xHex().String()
}

func (a EthAddress) HexString() string {
	return hex.EncodeToString(a[:])
}

func (a *EthAddress) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseEthAddress(s)
	if err != nil {
		return err
	}
	*a = *parsed
	return nil
}

func (a EthAddress) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// Scan implements sql.Scanner
func (a *EthAddress) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		return nil

	case string:
		addr, err := ethtypes.NewAddress(src)
		if err != nil {
			return err
		}
		*a = EthAddress(*addr)
		return nil

	case []byte:
		switch len(src) {
		case 20:
			copy((*a)[:], src)
		case 40, 42 /* with 0x */ :
			addr, err := ethtypes.NewAddress((string)(src))
			if err != nil {
				return err
			}
			*a = EthAddress(*addr)
		default:
			return i18n.NewError(context.Background(), pldmsgs.MsgTypesRestoreFailed, src, a)
		}
		return nil

	default:
		return i18n.NewError(context.Background(), pldmsgs.MsgTypesRestoreFailed, src, a)
	}

}

// Value implements sql.Valuer
func (a EthAddress) Value() (driver.Value, error) {
	// no prefix - always 40 chars
	return hex.EncodeToString(a[:]), nil
}
