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

package guid

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/kaleido-io/paladin/kata/internal/msgs"
)

// We create 20 byte identifiers for our GUIDs which makes them Eth-like in
// their storage, convenient for use in on-chain structures as an "address"
// if required, and easily usable as an addresses if required.
//
// We pad the first 4 bytes with an eyecatcher, so these random values do not mask the fact that
// this random value is an address.
// Then the last 16 bytes are a UUIDv4
type GUID [20]byte

// First 4 bytes of the Keccak-256 hash of "EthStyleGUID(UUIDv4)"
// a7522ea1d712b0fdfacd70ee4a45ae2ecfed532d616cca9956ac6bf258e4b497
// ^^^^^^^^
var ethStyleUUIDv4EyeCatcher = [4]byte{0xa7, 0x52, 0x2e, 0xa1}

func NewGUID() GUID {
	return NewEthStyleGUID_UUIDv4()
}

func NewEthStyleGUID_UUIDv4() GUID {
	uuidV4 := uuid.New()
	var id GUID
	copy(id[0:4], ethStyleUUIDv4EyeCatcher[:])
	copy(id[4:20], uuidV4[:])
	return id
}

// Parse a string
func ParseGUID(ctx context.Context, s string) (*GUID, error) {
	addr, err := ethtypes.NewAddress(s)
	if err != nil {
		return nil, err
	}
	id := (*GUID)(addr)
	if err := id.CheckValid(ctx); err != nil {
		return nil, err
	}
	return id, nil
}

func MustParseGUID(s string) GUID {
	addr, err := ethtypes.NewAddress(s)
	if err != nil {
		panic(err)
	}
	return GUID(*addr)
}

// Natural string representation is lower case hex, no 0x prefix
func (id *GUID) String() string {
	if id == nil {
		return ""
	}
	return (ethtypes.AddressPlainHex)(*id).String()
}

// Check eyecatcher is valid (nil safe)
func (id *GUID) CheckValid(ctx context.Context) error {
	if id == nil {
		return i18n.NewError(ctx, msgs.MsgGUIDNil)
	}
	if !bytes.Equal(id[0:4], ethStyleUUIDv4EyeCatcher[:]) {
		return i18n.NewError(ctx, msgs.MsgGUIDInvalidEyeCatcher, hex.EncodeToString(id[0:4]))
	}
	if bytes.Equal(id[4:20], uuid.Nil[:]) {
		return i18n.NewError(ctx, msgs.MsgGUIDZero)
	}
	return nil
}

// Valid check (nil safe)
func (id *GUID) IsValid() bool {
	err := id.CheckValid(context.Background())
	return err == nil
}

// Nil safe UUID getter - no check on eyecatcher (returns all zeros for nil)
func (id *GUID) UUID() uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return (uuid.UUID)(id[4:20])
}

// Nil safe Address0xHex getter - no check on eyecatcher (returns all zeros for nil)
func (id *GUID) Address0xHex() ethtypes.Address0xHex {
	if id == nil {
		return ethtypes.Address0xHex{}
	}
	return (ethtypes.Address0xHex)(id[:])
}
