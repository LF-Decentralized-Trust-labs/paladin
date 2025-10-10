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
	"sync"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix("PD04", "Paladin Static Registry")
	})
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Generic PD0400XX
	MsgInvalidRegistryConfig = pde("PD040001", "Invalid registry configuration")
	MsgFunctionUnsupported   = pde("PD040002", "Function not supported")
)
