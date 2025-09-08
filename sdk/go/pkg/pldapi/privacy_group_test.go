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

package pldapi

import (
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/stretchr/testify/require"
)

func TestPrivacyGroupABISchema(t *testing.T) {
	require.NotNil(t, PrivacyGroupABISchema())
}

func TestKeyValueStringProperties(t *testing.T) {

	m := map[string]string{
		"aaa": "AAA",
		"bbb": "BBB",
		"ccc": "CCC",
		"ddd": "DDD",
		"eee": "EEE",
		"fff": "FFF",
	}
	p := NewKeyValueStringProperties(m)
	require.Equal(t, KeyValueStringProperties{
		{"aaa", "AAA"},
		{"bbb", "BBB"},
		{"ccc", "CCC"},
		{"ddd", "DDD"},
		{"eee", "EEE"},
		{"fff", "FFF"},
	}, p)

	require.Equal(t, m, p.Map())

	require.JSONEq(t, `{
		"genesisSalt": "0xe29c67c5f341d94da089d78460f8080e76e75e07d714a030037d5ace8e1626b8",
		"name": "pg1",
		"members": [ "me@node1", "you@node2" ],
		"properties": [
			{"key": "aaa", "value": "AAA"},
			{"key": "bbb", "value": "BBB"},
			{"key": "ccc", "value": "CCC"},
			{"key": "ddd", "value": "DDD"},
			{"key": "eee", "value": "EEE"},
			{"key": "fff", "value": "FFF"}
		],
		"configuration": [
			{"key": "conf1", "value": "value1"}
		]
	}`, pldtypes.JSONString((&PrivacyGroup{
		GenesisSalt:   pldtypes.MustParseBytes32("e29c67c5f341d94da089d78460f8080e76e75e07d714a030037d5ace8e1626b8"),
		Name:          "pg1",
		Members:       []string{"me@node1", "you@node2"},
		Properties:    m,
		Configuration: map[string]string{"conf1": "value1"},
	}).GenesisStateData()).Pretty())

}
