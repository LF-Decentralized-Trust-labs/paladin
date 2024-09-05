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

package tktypes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrivateIdentityLocator(t *testing.T) {
	pil1 := PrivateIdentityLocator("i.am@this_node.org1")
	assert.Equal(t, "i.am@this_node.org1", pil1.String())
	pil1a, err := pil1.FullyQualified(context.Background(), "anything")
	assert.NoError(t, err)
	assert.Equal(t, "i.am@this_node.org1", pil1a.String())
	identity, node, err := pil1.Validate(context.Background(), "", true)
	assert.NoError(t, err)
	assert.Equal(t, "i.am", identity)
	assert.Equal(t, "this_node.org1", node)

	node, err = pil1.Node(context.Background(), true)
	assert.NoError(t, err)
	assert.Equal(t, "this_node.org1", node)

	identity, err = pil1.Identity(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "i.am", identity)

	pil2 := PrivateIdentityLocator("me")
	identity, node, err = pil2.Validate(context.Background(), "", true)
	assert.NoError(t, err)
	assert.Equal(t, "me", identity)
	assert.Empty(t, "", node)

	pil3 := PrivateIdentityLocator("me@")
	identity, node, err = pil3.Validate(context.Background(), "localnode", true)
	assert.NoError(t, err)
	assert.Equal(t, "me", identity)
	assert.Equal(t, "localnode", node)
	pil3a, err := pil3.FullyQualified(context.Background(), "localnode")
	assert.NoError(t, err)
	assert.Equal(t, "me@localnode", pil3a.String())

	type pilBox struct {
		ID PrivateIdentityLocator `json:"id"`
	}
	var pb1 pilBox
	err = json.Unmarshal([]byte(`{"id":"me@here"}`), &pb1)
	assert.NoError(t, err)
	identity, node, err = pb1.ID.Validate(context.Background(), "", true)
	assert.NoError(t, err)
	assert.Equal(t, "me", identity)
	assert.Equal(t, "here", node)

	jb, err := json.Marshal(&pb1)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"id":"me@here"}`, string(jb))
}

func TestPrivateIdentityLocatorErrors(t *testing.T) {
	_, _, err := PrivateIdentityLocator("@").Validate(context.Background(), "", false)
	assert.Regexp(t, "PD020005", err)

	_, _, err = PrivateIdentityLocator("me@some@where").Validate(context.Background(), "", false)
	assert.Regexp(t, "PD020006", err)

	_, _, err = PrivateIdentityLocator("me@_").Validate(context.Background(), "", false)
	assert.Regexp(t, "PD020005", err)

	_, err = PrivateIdentityLocator("_@").FullyQualified(context.Background(), "")
	assert.Regexp(t, "PD020005", err)
}
