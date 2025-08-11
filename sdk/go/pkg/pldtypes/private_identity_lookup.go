// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pldtypes

import (
	"context"
	"fmt"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/pldmsgs"
)

// The locator for private identities is split into two parts separate by an `@` symbol
// - A lookup for the identity, which can only be resolved by the node that owns that identity
// - A lookup for the node, which will be resolved locally to determine which node can resolve the identity
type PrivateIdentityLocator string

func (pil PrivateIdentityLocator) String() string {
	return string(pil)
}

func (pil PrivateIdentityLocator) Validate(ctx context.Context, defaultNode string, allowEmptyNode bool) (identity string, node string, err error) {
	parts := strings.Split(string(pil), "@")
	switch len(parts) {
	case 1:
		identity = parts[0]
	case 2:
		identity = parts[0]
		node = parts[1]
	default:
		return "", "", i18n.NewError(ctx, pldmsgs.MsgTypesPrivateIdentityLocatorInvalid, node)
	}
	if err := ValidateSafeCharsStartEndAlphaNum(ctx, identity, DefaultNameMaxLen, "identity"); err != nil {
		return "", "", i18n.WrapError(ctx, err, pldmsgs.MsgTypesPrivateIdentityLocatorInvalid, pil)
	}
	if node == "" {
		node = defaultNode
	}
	if node == "" /* 2nd check with any default applied */ {
		if !allowEmptyNode {
			return "", "", i18n.WrapError(ctx, err, pldmsgs.MsgTypesPrivateIdentityReqFullyQualified, pil)
		}
	} else {
		if err := ValidateSafeCharsStartEndAlphaNum(ctx, node, DefaultNameMaxLen, "node"); err != nil {
			return "", "", i18n.WrapError(ctx, err, pldmsgs.MsgTypesPrivateIdentityLocatorInvalid, pil)
		}
	}
	return identity, node, nil
}

func (pil PrivateIdentityLocator) Identity(ctx context.Context) (string, error) {
	identity, _, err := pil.Validate(ctx, "", true)
	return identity, err
}

func (pil PrivateIdentityLocator) Node(ctx context.Context, allowEmptyNode bool) (string, error) {
	_, node, err := pil.Validate(ctx, "", allowEmptyNode)
	return node, err
}

func (pil PrivateIdentityLocator) FullyQualified(ctx context.Context, defaultNode string) (PrivateIdentityLocator, error) {
	identity, node, err := pil.Validate(ctx, defaultNode, false)
	if err != nil {
		return "", err
	}
	return PrivateIdentityLocator(fmt.Sprintf("%s@%s", identity, node)), nil
}
