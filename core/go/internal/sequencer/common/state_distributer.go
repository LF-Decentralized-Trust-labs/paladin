/*
 * Copyright © 2025 Kaleido, Inc.
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

package common

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/core/internal/components"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
)

// This is the subset of the StateDistributer interface from "github.com/kaleido-io/paladin/core/internal/statedistribution"
// Here we define the subset that we rely on in this package

type StateDistributer interface {
	DistributeStates(ctx context.Context, stateDistributions []*components.StateDistribution)
}

// TODO don't like this name
type StateIntegration interface {
	// WriteLockAndDistributeStatesForTransaction is a method that writes a lock to the state and distributes the states for a transaction
	WriteLockAndDistributeStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error
}

type FakeStateIntegrationForTesting struct {
}

func (f *FakeStateIntegrationForTesting) WriteLockAndDistributeStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {
	return nil
}

// interface for existing implementation  in core/go/internal/privatetxnmgr/state_distribution_builder.go
type StateDistributionBuilder interface {
	Build(ctx context.Context, txn *components.PrivateTransaction) (sds *components.StateDistributionSet, err error)
}

type stateIntegration struct {
	stateDistributer         StateDistributer
	components               components.AllComponents
	domainAPI                components.DomainSmartContract
	domainContext            components.DomainContext
	stateDistributionBuilder StateDistributionBuilder
}

func (s *stateIntegration) WriteLockAndDistributeStatesForTransaction(ctx context.Context, txn *components.PrivateTransaction) error {

	//Write output states
	if txn.PostAssembly.OutputStatesPotential != nil && txn.PostAssembly.OutputStates == nil {
		readTX := s.components.Persistence().DB() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)
		err := s.domainAPI.WritePotentialStates(s.domainContext, readTX, txn)
		if err != nil {
			//Any error from WritePotentialStates is likely to be caused by an invalid init or assemble of the transaction
			// which is most likely a programming error in the domain or the domain manager or the sequencer
			errorMessage := fmt.Sprintf("Failed to write potential states: %s", err)
			log.L(ctx).Error(errorMessage)
			//TODO abort
			return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)
		} else {
			log.L(ctx).Debugf("Potential states written %s", s.domainContext.Info().ID)
		}
	}

	//Lock input states
	if len(txn.PostAssembly.InputStates) > 0 {
		readTX := s.components.Persistence().DB() // no DB transaction required here for the reads from the DB (writes happen on syncpoint flusher)

		err := s.domainAPI.LockStates(s.domainContext, readTX, txn)
		if err != nil {
			errorMessage := fmt.Sprintf("Failed to lock states: %s", err)
			log.L(ctx).Error(errorMessage)
			return i18n.NewError(ctx, msgs.MsgPrivateTxManagerInternalError, errorMessage)

		} else {
			log.L(ctx).Debugf("Input states locked %s: %s", s.domainContext.Info().ID, txn.PostAssembly.InputStates[0].ID)
		}
	}

	//Distribute states
	sds, err := s.stateDistributionBuilder.Build(ctx, txn)
	if err != nil {
		log.L(ctx).Errorf("Error getting state distributions: %s", err)
	}

	//distribute state data to remote nodes because they will need that data to a) endorse this, and future transactions and b) to assemble future transactions
	s.stateDistributer.DistributeStates(ctx, sds.Remote)

	return nil

}
