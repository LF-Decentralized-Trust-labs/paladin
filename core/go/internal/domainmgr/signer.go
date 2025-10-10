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
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
)

type domainSigner struct {
	dm *domainManager
}

func (ds *domainSigner) getDomainCheckSupport(ctx context.Context, algorithm string) (*domain, int, error) {
	domainName := strings.SplitN(strings.TrimPrefix(strings.ToLower(algorithm), "domain:"), ":", 2)[0]
	domain, err := ds.dm.getDomainByName(ctx, domainName)
	if err != nil {
		return nil, -1, err
	}

	// Because private keys are pushed to a domain that enables signing, the config of the node
	// set by the administrator must explicitly enable signing for a domain hosted within Paladin
	if !domain.conf.AllowSigning {
		return nil, -1, i18n.NewError(ctx, msgs.MsgDomainSigningDisabled, domainName)
	}

	// And the algorithm must have been registered during init
	var keyLen int32
	var algoFound bool
	domainConf := domain.Configuration()
	algoMap := domainConf.SigningAlgorithms
	if algoMap != nil {
		keyLen, algoFound = algoMap[algorithm]
	}
	if !algoFound {
		return nil, -1, i18n.NewError(ctx, msgs.MsgDomainSigningAlgorithmNotSupported, domainName, algorithm)
	}
	return domain, int(keyLen), nil

}

func (ds *domainSigner) GetMinimumKeyLen(ctx context.Context, algorithm string) (int, error) {
	_, keyLen, err := ds.getDomainCheckSupport(ctx, algorithm)
	return keyLen, err
}

func (ds *domainSigner) GetVerifier(ctx context.Context, algorithm string, verifierType string, privateKey []byte) (verifier string, err error) {
	domain, _, err := ds.getDomainCheckSupport(ctx, algorithm)
	if err == nil {
		verifier, err = domain.getVerifier(ctx, algorithm, verifierType, privateKey)
	}
	return
}

func (ds *domainSigner) Sign(ctx context.Context, algorithm string, payloadType string, privateKey []byte, payload []byte) (signature []byte, err error) {
	domain, _, err := ds.getDomainCheckSupport(ctx, algorithm)
	if err == nil {
		signature, err = domain.sign(ctx, algorithm, payloadType, privateKey, payload)
	}
	return
}
