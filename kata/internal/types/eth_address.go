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

package types

import (
	"context"
	"database/sql/driver"
	"encoding/hex"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

// EthAddress is an SQL serializable version of ethtypes.Address0xHex
type EthAddress [20]byte

func (a *EthAddress) Address0xHex() *ethtypes.Address0xHex {
	return (*ethtypes.Address0xHex)(a)
}

func (a *EthAddress) String() string {
	return a.Address0xHex().String()
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
			return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, a)
		}
		return nil

	default:
		return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, a)
	}

}

// Value implements sql.Valuer
func (a *EthAddress) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	// no prefix - always 40 chars
	return hex.EncodeToString(a[:]), nil
}
