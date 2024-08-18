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
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"golang.org/x/text/language"
)

var registered = false
var ffe = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	if !registered {
		i18n.RegisterPrefix("PD02", "Paladin Toolkit")
		registered = true
	}
	return i18n.FFE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Generic PD0200XX
	MsgContextCanceled = ffe("PD020000", "Context canceled")

	// Inflight PD0201XX
	MsgInflightRequestCancelled = ffe("PD020100", "Request cancelled after %s")

	// Config PD0202XX
	MsgConfigFileMissing               = ffe("PD020200", "Config file not found at path: %s")
	MsgConfigFileReadError             = ffe("PD020201", "Failed to read config file %s with error: %s")
	MsgConfigFileParseError            = ffe("PD020202", "Failed to parse config file %s with error: %s")
	MsgConfigFileMissingMandatoryValue = ffe("PD020203", "Mandatory config field %s missing ")
)
