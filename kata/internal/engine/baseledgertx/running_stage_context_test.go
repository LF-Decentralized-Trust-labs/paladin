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

package baseledgertx

import (
	"context"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	baseTypes "github.com/kaleido-io/paladin/kata/internal/engine/types"
	"github.com/kaleido-io/paladin/kata/mocks/enginemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRunningStageContext(t *testing.T) {
	imtx := NewTestInMemoryTxState(t)
	newSubStatus := baseTypes.BaseTxSubStatusConfirmed
	testRunningStageContext := NewRunningStageContext(context.Background(), baseTypes.InFlightTxStageReceipting, "", imtx)
	assert.Empty(t, testRunningStageContext.SubStatus)
	assert.Nil(t, testRunningStageContext.StageOutputsToBePersisted)
	testRunningStageContext.SetSubStatus(newSubStatus)
	assert.Equal(t, newSubStatus, testRunningStageContext.SubStatus)
	testRunningStageContext.SetNewPersistenceUpdateOutput()
	assert.NotNil(t, testRunningStageContext.StageOutputsToBePersisted)

	assert.Empty(t, testRunningStageContext.StageOutputsToBePersisted.HistoryUpdates)
	testRunningStageContext.StageOutputsToBePersisted.AddSubStatusAction(baseTypes.BaseTxActionRetrieveGasPrice, fftypes.JSONAnyPtr("info"), fftypes.JSONAnyPtr("error"))
	assert.Equal(t, 1, len(testRunningStageContext.StageOutputsToBePersisted.HistoryUpdates))

	mTS := enginemocks.NewTransactionStore(t)
	mTS.On("AddSubStatusAction", mock.Anything, mock.Anything, newSubStatus, baseTypes.BaseTxActionRetrieveGasPrice, fftypes.JSONAnyPtr("info"), fftypes.JSONAnyPtr("error"), mock.Anything).Return(nil).Once()
	testRunningStageContext.StageOutputsToBePersisted.HistoryUpdates[0](mTS)

}
