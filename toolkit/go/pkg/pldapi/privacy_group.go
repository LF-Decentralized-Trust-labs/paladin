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

package pldapi

import (
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

type PrivacyGroup struct {
	ID              tktypes.HexBytes  `docstruct:"PrivacyGroup" json:"id"`
	Domain          string            `docstruct:"PrivacyGroup" json:"domain"`
	Created         tktypes.Timestamp `docstruct:"PrivacyGroup" json:"created"`
	Schema          tktypes.HexBytes  `docstruct:"PrivacyGroup" json:"schema"`
	SchemaSignature string            `docstruct:"PrivacyGroup" json:"schemaSignature"`
	Originator      string            `docstruct:"PrivacyGroup" json:"originator"`
	Members         []string          `docstruct:"PrivacyGroup" json:"members"`
	Properties      tktypes.RawJSON   `docstruct:"PrivacyGroup" json:"properties"`
}

type PrivacyGroupInput struct {
	Domain     string          `docstruct:"PrivacyGroup" json:"domain"`
	Members    []string        `docstruct:"PrivacyGroup" json:"members"`
	Properties tktypes.RawJSON `docstruct:"PrivacyGroup" json:"properties"`
}
