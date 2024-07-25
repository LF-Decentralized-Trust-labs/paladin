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

package domaintestbed

import (
	"github.com/kaleido-io/paladin/kata/pkg/proto"
)

type TestBed interface {
	ToDomain(req ToDomainRequest) (res ToDomainResponse, err error)
	FromDomainUDS() string
}

type ToDomainRequestType int

const (
	CONFIG ToDomainRequestType = iota
	INIT
	DEPLOY
	PLAN
	ASSEMBLE
	PREPARE
)

type ToDomainRequest struct {
	Type                ToDomainRequestType
	ConfigureDomain     *proto.ConfigureDomainRequest
	InitDomain          *proto.InitDomainRequest
	PrepareDeploy       *proto.PrepareDeployTransactionRequest
	PlanTransaction     *proto.PlanTransactionRequest
	AssembleTransaction *proto.AssembleTransactionRequest
	PrepareTransaction  *proto.PrepareDeployTransactionRequest
}

type ToDomainResponse struct {
	Type                ToDomainRequestType
	ConfigureDomain     *proto.ConfigureDomainResponse
	InitDomain          *proto.InitDomainResponse
	PrepareDeploy       *proto.PrepareDeployTransactionResponse
	PlanTransaction     *proto.PlanTransactionResponse
	AssembleTransaction *proto.AssembleTransactionResponse
	PrepareTransaction  *proto.PrepareDeployTransactionResponse
}

// Creating a testbed starts two gRPC servers:
// 1) For making synchronous calls to the domain
// 2) For the domain to make synchronous calls back to the test-bed during those calls
// The testbed implements (2)
func NewTestBed() TestBed {
	return nil
}
