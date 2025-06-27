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

	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

const notoPrefix = "PD20"

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix(notoPrefix, "Noto Domain")
	})
	if !strings.HasPrefix(key, notoPrefix) {
		panic(fmt.Errorf("must have prefix '%s': %s", notoPrefix, key))
	}
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	MsgUnexpectedConfigType               = pde("PD200000", "Unexpected config type: %s")
	MsgUnknownFunction                    = pde("PD200001", "Unknown function: %s")
	MsgUnexpectedFunctionSignature        = pde("PD200002", "Unexpected signature for function '%s': expected=%s actual=%s")
	MsgUnexpectedSchema                   = pde("PD200003", "Unexpected schema: %s")
	MsgInvalidListInput                   = pde("PD200004", "Invalid item in list %s[%d] (%s): %s")
	MsgInsufficientFunds                  = pde("PD200005", "Insufficient funds (available=%s)")
	MsgInvalidStateData                   = pde("PD200006", "State data %s is invalid: %s")
	MsgParameterRequired                  = pde("PD200007", "Parameter '%s' is required")
	MsgParameterGreaterThanZero           = pde("PD200008", "Parameter '%s' must be greater than 0")
	MsgMintOnlyNotary                     = pde("PD200009", "Mint can only be initiated by notary: expected=%s actual=%s")
	MsgErrorVerifyingAddress              = pde("PD200011", "Error verifying '%s' address")
	MsgInvalidInputs                      = pde("PD200012", "Invalid inputs to '%s': %v")
	MsgInvalidAmount                      = pde("PD200013", "Invalid amount for '%s': expected=%s actual=%s")
	MsgUnknownDomainVariant               = pde("PD200014", "Unknown domain variant: %s")
	MsgAttestationNotFound                = pde("PD200015", "Did not find '%s' attestation")
	MsgAttestationUnexpected              = pde("PD200016", "Attestation for '%s' did not match expected lookup: expected=%s actual=%s")
	MsgSignatureDoesNotMatch              = pde("PD200017", "Signature for '%s' did not match: expected=%s actual=%s")
	MsgStateWrongOwner                    = pde("PD200018", "State '%s' is not owned by '%s'")
	MsgUnrecognizedEndorsement            = pde("PD200019", "Unrecognized endorsement request: %s")
	MsgDuplicateStateInList               = pde("PD200020", "Duplicate state in list %s[%d] (%s)")
	MsgNotImplemented                     = pde("PD200022", "Not implemented")
	MsgInvalidDelegate                    = pde("PD200023", "Invalid delegate: %s")
	MsgNoDomainReceipt                    = pde("PD200024", "Not implemented. See state receipt for coin transfers")
	MsgBurnNotAllowed                     = pde("PD200025", "Burn is not enabled")
	MsgNoStatesSpecified                  = pde("PD200026", "No states were specified")
	MsgUnlockNotAllowed                   = pde("PD200027", "Cannot unlock states owned by '%s'")
	MsgLockIDNotFound                     = pde("PD200028", "Lock ID not found")
	MsgMissingStateData                   = pde("PD200029", "Missing state data for one or more states: %s")
	MsgLockNotAllowed                     = pde("PD200030", "Lock is not enabled")
	MsgUnlockOnlyCreator                  = pde("PD200031", "Only the lock creator can perform unlock: expected=%s actual=%s")
	MsgErrorValidateInitCallTxSpec        = pde("PD200032", "Failed to validate init call transaction spec. %s")
	MsgErrorValidateExecCallTxSpec        = pde("PD200033", "Failed to validate execute call transaction spec. %s")
	MsgErrorGetAccountBalance             = pde("PD200034", "Failed to get account balance. %s")
	MsgErrorHandlerImplementationNotFound = pde("PD200035", "Handler implementation not found. %s")
)
