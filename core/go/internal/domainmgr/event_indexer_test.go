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

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/core/pkg/blockindexer"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

func TestSolidityEventSignatures(t *testing.T) {
	// We don't expect this to change without our knowledge.
	// We tolerate it changing between versions of firefly-signer (used only in memory), but it's important we understand why if it does.
	//
	// We don't store it as a constant because we're reliant on us and blockindexer calculating it identically (we use the same lib).
	//
	// The standard solidity signature is insufficient, as it doesn't include variable names, or the indexed-ness of fields
	assert.Equal(t, "event PaladinNewSmartContract_V0(bytes32 indexed txId, address indexed domain, bytes data)", eventSolSig_PaladinNewSmartContract_V0)
}

func TestEventIndexingWithDB(t *testing.T) {

	ctx, dm, tp, done := newTestDomain(t, true /* real DB */, goodDomainConf())
	defer done()

	deployTX := uuid.New()
	contractAddr := tktypes.EthAddress(tktypes.RandBytes(20))

	txNotified := make(chan struct{})
	go func() {
		defer close(txNotified)
		sc, err := dm.WaitForDeploy(ctx, deployTX)
		assert.NoError(t, err)
		assert.Equal(t, contractAddr, sc.Address())
	}()

	// Index an event indicating deployment of a new smart contract instance
	var pc blockindexer.PostCommit
	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) (err error) {
		pc, err = dm.eventIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinNewSmartContract_V0,
					Address:           contractAddr,
					IndexedEvent: &blockindexer.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  tktypes.NewBytes32FromSlice(tktypes.RandBytes(32)),
						Signature:        eventSig_PaladinNewSmartContract_V0,
					},
					Data: tktypes.RawJSON(`{
						"txId": "` + tktypes.Bytes32UUIDFirst16(deployTX).String() + `",
						"domain": "` + tp.d.factoryContractAddress.String() + `",
						"data": "0xfeedbeef"
					}`),
				},
			},
		})
		return err
	})
	assert.NoError(t, err)
	assert.NotNil(t, pc)
	pc()

	// Lookup the instance against the domain
	psc, err := dm.GetSmartContractByAddress(ctx, contractAddr)
	assert.NoError(t, err)
	dc := psc.(*domainContract)
	assert.Equal(t, &PrivateSmartContract{
		DeployTX:      deployTX,
		DomainAddress: *tp.d.factoryContractAddress,
		Address:       contractAddr,
		ConfigBytes:   []byte{0xfe, 0xed, 0xbe, 0xef},
	}, dc.info)
	assert.Equal(t, contractAddr, psc.Address())
	assert.Equal(t, "test1", psc.Domain().Name())
	assert.Equal(t, "0xfeedbeef", psc.ConfigBytes().String())
	assert.Equal(t, tp.d.factoryContractAddress, psc.Domain().Address())

	// Get cached
	psc2, err := dm.GetSmartContractByAddress(ctx, contractAddr)
	assert.NoError(t, err)
	assert.Equal(t, psc, psc2)

	<-txNotified
}

func TestEventIndexingBadEvent(t *testing.T) {

	ctx, dm, _, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectCommit()
	})
	defer done()

	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) error {
		_, err := dm.eventIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinNewSmartContract_V0,
					Data: tktypes.RawJSON(`{
						 "data": "cannot parse this"
					 }`),
				},
			},
		})
		return err
	})
	assert.NoError(t, err)

}

func TestEventIndexingInsertError(t *testing.T) {

	ctx, dm, tp, done := newTestDomain(t, false, goodDomainConf(), func(mc *mockComponents) {
		mc.domainStateInterface.On("EnsureABISchemas", mock.Anything).Return(nil, nil)
		mc.db.ExpectBegin()
		mc.db.ExpectExec("INSERT").WillReturnError(fmt.Errorf("pop"))
		mc.db.ExpectRollback()
	})
	defer done()

	contractAddr := tktypes.EthAddress(tktypes.RandBytes(20))
	deployTX := uuid.New()
	err := dm.persistence.DB().Transaction(func(tx *gorm.DB) error {
		_, err := dm.eventIndexer(ctx, tx, &blockindexer.EventDeliveryBatch{
			StreamID:   uuid.New(),
			StreamName: "name_given_by_component_mgr",
			BatchID:    uuid.New(),
			Events: []*blockindexer.EventWithData{
				{
					SoliditySignature: eventSolSig_PaladinNewSmartContract_V0,
					Address:           contractAddr,
					IndexedEvent: &blockindexer.IndexedEvent{
						BlockNumber:      12345,
						TransactionIndex: 0,
						LogIndex:         0,
						TransactionHash:  tktypes.NewBytes32FromSlice(tktypes.RandBytes(32)),
						Signature:        eventSig_PaladinNewSmartContract_V0,
					},
					Data: tktypes.RawJSON(`{
						"txId": "` + tktypes.Bytes32UUIDFirst16(deployTX).String() + `",
						"domain": "` + tp.d.factoryContractAddress.String() + `",
						"data": "0xfeedbeef"
					}`),
				},
			},
		})
		return err
	})
	assert.Regexp(t, "pop", err)

}
