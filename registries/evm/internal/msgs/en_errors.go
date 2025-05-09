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

	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"golang.org/x/text/language"
)

var registered sync.Once
var pde = func(key, translation string, statusHint ...int) i18n.ErrorMessageKey {
	registered.Do(func() {
		i18n.RegisterPrefix("PD06", "Paladin EVM Registry")
	})
	return i18n.PDE(language.AmericanEnglish, key, translation, statusHint...)
}

var (
	// Generic PD0600XX
	MsgInvalidRegistryConfig  = pde("PD060001", "Invalid registry configuration")
	MsgInvalidRegistryEvent   = pde("PD060002", "Invalid registry event %+v")
	MsgMissingContractAddress = pde("PD060003", "contractAddress is required in registry config")
)
