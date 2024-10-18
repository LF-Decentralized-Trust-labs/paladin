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

package keymanager

import (
	"context"
	"regexp"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/internal/msgs"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/signer"
	"github.com/kaleido-io/paladin/toolkit/pkg/signerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type wallet struct {
	name          string
	keySelector   *regexp.Regexp
	signingModule signer.SigningModule
}

func (km *keyManager) newWallet(ctx context.Context, walletConf *pldconf.WalletConfig) (w *wallet, err error) {

	w = &wallet{
		name: walletConf.Name,
	}

	if err := tktypes.ValidateSafeCharsStartEndAlphaNum(ctx, w.name, tktypes.DefaultNameMaxLen, "name"); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgKeyManagerInvalidConfig, w.name)
	}

	w.keySelector, err = regexp.Compile(confutil.StringNotEmpty(&walletConf.KeySelector, pldconf.WalletDefaults.KeySelector))
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgKeyManagerInvalidKeySelector, w.name)
	}

	signerType := confutil.StringNotEmpty(&walletConf.SignerType, pldconf.WalletDefaults.SignerType)
	if signerType != pldconf.WalletSignerTypeEmbedded {
		// Until we support remoting of the signer API over HTTP/WebSockets etc.
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerInvalidWalletSignerType, signerType, w.name)
	}

	w.signingModule, err = signer.NewSigningModule(ctx, (*signerapi.ConfigNoExt)(walletConf.Signer))
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgKeyManagerEmbeddedSignerFailInit, w.name)
	}

	return w, nil

}

func (km *keyManager) selectWallet(ctx context.Context, identifier string) (*wallet, error) {
	for i, w := range km.walletsOrdered {
		if w.keySelector.MatchString(identifier) {
			log.L(ctx).Infof("identifier %s matched by wallet %d (%s)", identifier, i, w.name)
			return w, nil
		}
	}
	return nil, i18n.NewError(ctx, msgs.MsgKeyManagerNoWalletMatch, identifier)
}

func (km *keyManager) getWalletByName(ctx context.Context, walletName string) (*wallet, error) {
	w := km.walletsByName[walletName]
	if w == nil {
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerWalletNotConfigured, walletName)
	}
	return w, nil
}

func (km *keyManager) getWalletList() []*pldapi.WalletInfo {
	walletNames := make([]*pldapi.WalletInfo, len(km.walletsOrdered))
	for i, w := range km.walletsOrdered {
		walletNames[i] = &pldapi.WalletInfo{
			Name:        w.name,
			KeySelector: w.keySelector.String(),
		}
	}
	return walletNames
}

func (w *wallet) resolveKeyAndVerifier(ctx context.Context, mapping *pldapi.KeyMappingWithPath, algorithm, verifierType string) (*pldapi.KeyMappingAndVerifier, error) {

	req := &signerapi.ResolveKeyRequest{
		Attributes: map[string]string{},
		RequiredIdentifiers: []*signerapi.PublicKeyIdentifierType{
			{Algorithm: algorithm, VerifierType: verifierType},
		},
		Path: []*signerapi.ResolveKeyPathSegment{},
	}

	for i := 0; i < (len(mapping.Path) - 1); i++ {
		req.Path = append(req.Path, &signerapi.ResolveKeyPathSegment{
			Name:  mapping.Path[i].Name,
			Index: uint64(mapping.Path[i].Index),
		})
	}
	leaf := mapping.Path[len(mapping.Path)-1]
	req.Name = leaf.Name
	req.Index = uint64(leaf.Index)
	res, err := w.signingModule.Resolve(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check the mapping input didn't have a different key handle, if the incoming mapping already had one on there
	if mapping.KeyHandle != "" && res.KeyHandle != mapping.KeyHandle {
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerKeyHandleNonDeterminism, w.name, res.KeyHandle, verifierType, mapping.KeyHandle)
	}

	if len(res.Identifiers) != 1 ||
		res.Identifiers[0].Algorithm != algorithm ||
		res.Identifiers[0].VerifierType != verifierType {
		log.L(ctx).Errorf("Invalid response from wallet '%s' expected[algorithm=%s,verifierType=%s] received: %+v", w.name, algorithm, verifierType, res)
		return nil, i18n.NewError(ctx, msgs.MsgKeyManagerInvalidResolveResponse, w.name)
	}

	mapping.KeyHandle = res.KeyHandle
	return &pldapi.KeyMappingAndVerifier{
		KeyMappingWithPath: mapping,
		Verifier: &pldapi.KeyVerifier{
			Algorithm: res.Identifiers[0].Algorithm,
			Type:      res.Identifiers[0].VerifierType,
			Verifier:  res.Identifiers[0].Verifier,
		},
	}, nil

}

func (w *wallet) sign(ctx context.Context, mapping *pldapi.KeyMappingAndVerifier, payloadType string, payload []byte) ([]byte, error) {
	log.L(ctx).Infof("Wallet '%s' signing %d bytes with keyHandle=%s algorithm=%s payloadType=%s", w.name, len(payload), mapping.KeyHandle, mapping.Verifier.Algorithm, payloadType)
	res, err := w.signingModule.Sign(ctx, &signerapi.SignRequest{
		KeyHandle:   mapping.KeyHandle,
		Algorithm:   mapping.Verifier.Algorithm,
		PayloadType: payloadType,
		Payload:     payload,
	})
	if err != nil {
		return nil, err
	}
	return res.Payload, nil
}
