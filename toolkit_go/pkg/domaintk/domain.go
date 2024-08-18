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
package domaintk

import (
	"context"

	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
)

type DomainAPI interface {
	ConfigureDomain(context.Context, *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error)
	InitDomain(context.Context, *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error)
	InitDeploy(context.Context, *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error)
	PrepareDeploy(context.Context, *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error)
	InitTransaction(context.Context, *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error)
	AssembleTransaction(context.Context, *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error)
	EndorseTransaction(context.Context, *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error)
	PrepareTransaction(context.Context, *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error)
}

type DomainCallbacks interface {
	FindAvailableStates(context.Context, *prototk.FindAvailableStatesRequest) (*prototk.FindAvailableStatesResponse, error)
}
