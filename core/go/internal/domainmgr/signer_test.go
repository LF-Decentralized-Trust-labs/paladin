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
	"context"
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainGetVerifierOk(t *testing.T) {
	conf := goodDomainConf()
	conf.SigningAlgorithms = map[string]int32{
		"domain:test1:algo1": 32,
	}

	td, done := newTestDomain(t, false, conf, mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())

	dm := td.dm
	tp := td.tp
	td.d.conf.AllowSigning = true

	keyLen, err := dm.GetSigner().GetMinimumKeyLen(td.ctx, "domain:test1:algo1")
	require.NoError(t, err)
	assert.Equal(t, 32, keyLen)

	tp.Functions.GetVerifier = func(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
		require.Equal(t, "domain:test1:algo1", req.Algorithm)
		require.Equal(t, "domain:test1:verifier_type", req.VerifierType)
		require.Equal(t, "private key", string(req.PrivateKey))
		return &prototk.GetVerifierResponse{
			Verifier: "verifier1",
		}, nil
	}
	verifier, err := dm.GetSigner().GetVerifier(td.ctx, "domain:test1:algo1", "domain:test1:verifier_type", []byte("private key"))
	require.NoError(t, err)
	assert.Equal(t, "verifier1", verifier)

	tp.Functions.Sign = func(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
		require.Equal(t, "domain:test1:algo1", req.Algorithm)
		require.Equal(t, "domain:test1:payload_type", req.PayloadType)
		require.Equal(t, "private key", string(req.PrivateKey))
		require.Equal(t, "payload", string(req.Payload))
		return &prototk.SignResponse{
			Payload: []byte("signed"),
		}, nil
	}
	signature, err := dm.GetSigner().Sign(td.ctx, "domain:test1:algo1", "domain:test1:payload_type", []byte("private key"), []byte("payload"))
	require.NoError(t, err)
	assert.Equal(t, "signed", string(signature))
}

func TestDomainGetVerifierErrors(t *testing.T) {
	conf := goodDomainConf()
	conf.SigningAlgorithms = map[string]int32{
		"domain:test1:algo1": 32,
	}

	td, done := newTestDomain(t, false, conf, mockSchemas())
	defer done()
	assert.Nil(t, td.d.initError.Load())
	dm := td.dm
	tp := td.tp

	_, err := dm.GetSigner().GetMinimumKeyLen(td.ctx, "domain:test2:algo1")
	assert.Regexp(t, "PD011600", err)

	_, err = dm.GetSigner().GetMinimumKeyLen(td.ctx, "domain:test1:algo2")
	assert.Regexp(t, "PD011643", err)

	td.d.conf.AllowSigning = true

	_, err = dm.GetSigner().GetMinimumKeyLen(td.ctx, "domain:test1:algo2")
	assert.Regexp(t, "PD011644", err)

	tp.Functions.GetVerifier = func(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err = dm.GetSigner().GetVerifier(td.ctx, "domain:test1:algo1", "domain:test1:verifier_type", []byte("private key"))
	assert.Regexp(t, "pop", err)

	tp.Functions.Sign = func(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err = dm.GetSigner().Sign(td.ctx, "domain:test1:algo1", "domain:test1:payload_type", []byte("private key"), []byte("payload"))
	assert.Regexp(t, "pop", err)
}
