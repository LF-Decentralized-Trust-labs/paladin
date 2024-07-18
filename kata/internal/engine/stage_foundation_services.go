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

package engine

import (
	"github.com/kaleido-io/paladin/kata/internal/statestore"
)

type MockTalariaInfo struct {
}

func (mti *MockTalariaInfo) IsCurrentNode(nodeID string) bool {
	return nodeID == "current-node"
}

func (mti *MockTalariaInfo) GetDispatchAddress(preferredAddresses []string) string {
	if len(preferredAddresses) > 0 {
		return preferredAddresses[0]
	}
	return ""
}

type TalariaInfo interface {
	IsCurrentNode(nodeID string) bool
	GetDispatchAddress(preferredAddresses []string) string
}

type StageFoundationService interface {
	TalariaInfo() TalariaInfo
	DependencyChecker() DependencyChecker
	StateStore() statestore.StateStore // TODO: filter out to only getters so setters can be coordinated efficiently like transactions
}

type PaladinStageFoundationService struct {
	dependencyChecker DependencyChecker
	stateStore        statestore.StateStore
	talariaInfo       TalariaInfo
}

func (psfs *PaladinStageFoundationService) DependencyChecker() DependencyChecker {
	return psfs.dependencyChecker
}

func (psfs *PaladinStageFoundationService) StateStore() statestore.StateStore {
	return psfs.stateStore
}

func (psfs *PaladinStageFoundationService) TalariaInfo() TalariaInfo {
	return psfs.talariaInfo
}
