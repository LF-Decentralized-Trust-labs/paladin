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

package syncpoints

import (
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/mocks/componentsmocks"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence/mockpersistence"
	"github.com/stretchr/testify/require"
)

type dependencyMocks struct {
	persistence  *mockpersistence.SQLMockProvider
	txMgr        *componentsmocks.TXManager
	transportMgr *componentsmocks.TransportManager
}

func newSyncPointsForTesting(t *testing.T) (*syncPoints, *dependencyMocks) {
	p, err := mockpersistence.NewSQLMockProvider()
	require.NoError(t, err)
	mocks := &dependencyMocks{
		persistence:  p,
		txMgr:        componentsmocks.NewTXManager(t),
		transportMgr: componentsmocks.NewTransportManager(t),
	}

	return &syncPoints{
		txMgr:        mocks.txMgr,
		transportMgr: mocks.transportMgr,
	}, mocks
}
