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

package testbed

import (
	"context"
	"fmt"

	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/ethclient"

	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/signerapi"
)

type ethClientKeyMgrShim struct {
	tb       *testbed
	resolved map[string]*pldapi.KeyMappingAndVerifier
}

func (e *ethClientKeyMgrShim) AddInMemorySigner(prefix string, signer signerapi.InMemorySigner) {
	panic("unimplemented")
}

func (e *ethClientKeyMgrShim) Close() {}

func (e *ethClientKeyMgrShim) ResolveKey(ctx context.Context, identifier string, algorithm string, verifierType string) (keyHandle string, verifier string, err error) {
	resolvedKey, err := e.tb.Components().KeyManager().ResolveKeyNewDatabaseTX(ctx, identifier, algorithm, verifierType)
	if err != nil {
		return "", "", err
	}
	e.resolved[fmt.Sprintf("%s|%s", resolvedKey.KeyHandle, algorithm)] = resolvedKey
	return resolvedKey.KeyHandle, resolvedKey.Verifier.Verifier, nil
}

func (e *ethClientKeyMgrShim) Sign(ctx context.Context, req *prototk.SignWithKeyRequest) (*prototk.SignWithKeyResponse, error) {
	mapping := e.resolved[fmt.Sprintf("%s|%s", req.KeyHandle, req.Algorithm)]
	if mapping == nil {
		return nil, fmt.Errorf("combination not resolved in this shim: keyHandle=%s, algorithm=%s", req.KeyHandle, req.Algorithm)
	}
	signedPayload, err := e.tb.c.KeyManager().Sign(ctx, mapping, req.PayloadType, req.Payload)
	if err != nil {
		return nil, err
	}
	return &prototk.SignWithKeyResponse{Payload: signedPayload}, nil
}

func (tb *testbed) EthClientKeyManagerShim() ethclient.KeyManager {
	return &ethClientKeyMgrShim{
		tb:       tb,
		resolved: make(map[string]*pldapi.KeyMappingAndVerifier),
	}
}
