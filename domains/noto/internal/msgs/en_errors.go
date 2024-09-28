// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package msgs

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

const notoPrefix = "PD20"

var registered sync.Once
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix(notoPrefix, "Noto Domain")
	})
	if !strings.HasPrefix(key, notoPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", notoPrefix, key))
	}
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	MsgUnexpectedConfigType        = ffe("PD200000", "Unexpected config type: %s")
	MsgUnknownFunction             = ffe("PD200001", "Unknown function: %s")
	MsgUnexpectedFunctionSignature = ffe("PD200002", "Unexpected signature for function '%s': expected=%s actual=%s")
	MsgUnknownSchema               = ffe("PD200003", "Unknown schema: %s")
	MsgInvalidListInput            = ffe("PD200004", "Invalid item in list %s[%d] (%s): %s")
	MsgInsufficientFunds           = ffe("PD200005", "Insufficient funds (available=%s)")
	MsgInvalidStateData            = ffe("PD200006", "State data %s is invalid: %s")
	MsgParameterRequired           = ffe("PD200007", "Parameter '%s' is required")
	MsgParameterGreaterThanZero    = ffe("PD200008", "Parameter '%s' must be greater than 0")
	MsgMintOnlyNotary              = ffe("PD200009", "Mint can only be initiated by notary")
	MsgErrorVerifyingAddress       = ffe("PD200011", "Error verifying '%s' address")
	MsgInvalidInputs               = ffe("PD200012", "Invalid inputs to '%s': %v")
	MsgInvalidAmount               = ffe("PD200013", "Invalid amount for '%s': expected=%s actual=%s")
	MsgUnknownDomainVariant        = ffe("PD200014", "Unknown domain variant: %s")
	MsgAttestationNotFound         = ffe("PD200015", "Did not find '%s' attestation")
	MsgAttestationUnexpected       = ffe("PD200016", "Attestation for '%s' did not match expected lookup: expected=%s actual=%s")
	MsgSignatureDoesNotMatch       = ffe("PD200017", "Signature for '%s' did not match: expected=%s actual=%s")
	MsgStateWrongOwner             = ffe("PD200018", "State '%s' is not owned by '%s'")
	MsgUnrecognizedEndorsement     = ffe("PD200019", "Unrecognized endorsement request: %s")
	MsgDuplicateStateInList        = ffe("PD200020", "Duplicate state in list %s[%d] (%s)")
	MsgUnknownEvent                = ffe("PD200021", "Unknown event: %s")
	MsgNotImplemented              = ffe("PD200022", "Not implemented")
	MsgInvalidDelegate             = ffe("PD200023", "Invalid delegate: %s")
)
