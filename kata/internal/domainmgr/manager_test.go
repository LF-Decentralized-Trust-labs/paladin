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

package main

import (
	"context"
	"testing"

	"github.com/kaleido-io/paladin/kata/internal/statestore"
	"github.com/kaleido-io/paladin/kata/pkg/persistence"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func newTestDomainManager(t *testing.T, conf *DomainManagerConfig) (context.Context, *domainManager, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	p, pDone, err := persistence.NewUnitTestPersistence(ctx)
	assert.NoError(t, err)

	stateStore := statestore.NewStateStore(ctx, &statestore.Config{}, p)

	dm := NewDomainManager(ctx, conf, stateStore, 12345)

	return ctx, dm.(*domainManager), func() {
		cancelCtx()
		stateStore.Close()
		pDone()
	}
}

func yamlNode(t *testing.T, s string) (n yaml.Node) {
	err := yaml.Unmarshal([]byte(s), &n)
	assert.NoError(t, err)
	return
}
