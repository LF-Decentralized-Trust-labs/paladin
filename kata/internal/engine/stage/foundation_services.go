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

package stage

import (
	"github.com/kaleido-io/paladin/kata/internal/statestore"
)

type MockNodeAndWalletLookUpService struct {
}

func (mti *MockNodeAndWalletLookUpService) IsCurrentNode(nodeID string) bool {
	return nodeID == "current-node"
}

func (mti *MockNodeAndWalletLookUpService) GetDispatchAddress(preferredAddresses []string) string {
	if len(preferredAddresses) > 0 {
		return preferredAddresses[0]
	}
	return ""
}

type NodeAndWalletLookUpService interface {
	IsCurrentNode(nodeID string) bool
	GetDispatchAddress(preferredAddresses []string) string
}

type StageFoundationService interface {
	NodeAndWallet() NodeAndWalletLookUpService
	DependencyChecker() DependencyChecker
	StateStore() statestore.StateStore // TODO: filter out to only getters so setters can be coordinated efficiently like transactions
}

type PaladinStageFoundationService struct {
	dependencyChecker   DependencyChecker
	stateStore          statestore.StateStore
	nodeAndWalletLookUp NodeAndWalletLookUpService
}

func (psfs *PaladinStageFoundationService) DependencyChecker() DependencyChecker {
	return psfs.dependencyChecker
}

func (psfs *PaladinStageFoundationService) StateStore() statestore.StateStore {
	return psfs.stateStore
}

func (psfs *PaladinStageFoundationService) NodeAndWallet() NodeAndWalletLookUpService {
	return psfs.nodeAndWalletLookUp
}

func NewPaladinStageFoundationService(dependencyChecker DependencyChecker,
	stateStore statestore.StateStore,
	nodeAndWalletLookUp NodeAndWalletLookUpService) StageFoundationService {
	return &PaladinStageFoundationService{
		dependencyChecker:   dependencyChecker,
		stateStore:          stateStore,
		nodeAndWalletLookUp: nodeAndWalletLookUp,
	}
}
