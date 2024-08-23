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

package domainmgr

// import (
// 	"fmt"
// 	"testing"

// 	"github.com/DATA-DOG/go-sqlmock"
// 	"github.com/kaleido-io/paladin/kata/pkg/types"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/mock"
// )

// func TestPrivateSmartContractQueryFail(t *testing.T) {

// 	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
// 		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
// 		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnError(fmt.Errorf("pop"))
// 	})
// 	defer done()

// 	_, err := dm.GetSmartContractByAddress(ctx, types.EthAddress(types.RandBytes(20)))
// 	assert.Regexp(t, "pop", err)

// }

// func TestPrivateSmartContractQueryNoResult(t *testing.T) {

// 	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
// 		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
// 		mc.db.ExpectQuery("SELECT.*private_smart_contracts").WillReturnRows(sqlmock.NewRows([]string{}))
// 	})
// 	defer done()

// 	_, err := dm.GetSmartContractByAddress(ctx, types.EthAddress(types.RandBytes(20)))
// 	assert.Regexp(t, "PD011609", err)

// }
