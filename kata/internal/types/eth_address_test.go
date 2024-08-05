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
	"testing"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/stretchr/testify/assert"
)

func TestEthAddress(t *testing.T) {

	a := (*EthAddress)(ethtypes.MustNewAddress("0xacA6D8Ba6BFf0fa5c8a06A58368CB6097285d5c5"))
	assert.Equal(t, "0xaca6d8ba6bff0fa5c8a06a58368cb6097285d5c5", a.String())

	var a1 *EthAddress
	err := a1.Scan(nil)
	assert.NoError(t, err)
	assert.Nil(t, a1)

	v1, err := a1.Value()
	assert.NoError(t, err)
	assert.Nil(t, v1)

	a2 := &EthAddress{}
	err = a2.Scan(a.String())
	assert.NoError(t, err)
	assert.Equal(t, a, a2)

	v2, err := a2.Value()
	assert.NoError(t, err)
	assert.Equal(t, a[:], v2)

	a3 := &EthAddress{}
	err = a3.Scan(([]byte)(a[:]))
	assert.NoError(t, err)
	assert.Equal(t, a, a3)

	a4 := &EthAddress{}
	err = a4.Scan(([]byte)(a.String()))
	assert.NoError(t, err)
	assert.Equal(t, a, a4)

	a5 := &EthAddress{}
	err = a5.Scan([]byte{0x01})
	assert.Regexp(t, "FF00105", err)

	a6 := &EthAddress{}
	err = a6.Scan(false)
	assert.Regexp(t, "FF00105", err)

	a7 := &EthAddress{}
	err = a7.Scan(([]byte)("!!aca6d8ba6bff0fa5c8a06a58368cb6097285d5"))
	assert.Regexp(t, "bad address", err)

	a8 := &EthAddress{}
	err = a8.Scan("!!aca6d8ba6bff0fa5c8a06a58368cb6097285d5")
	assert.Regexp(t, "bad address", err)
}
