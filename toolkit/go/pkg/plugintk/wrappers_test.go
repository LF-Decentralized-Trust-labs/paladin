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
package plugintk

import (
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/assert"
)

func TestCallPluginImpl(t *testing.T) {
	// Use domains for this test for convenience
	_, exerciser, funcs, _, _, done := setupDomainTests(t)
	defer done()

	// This is what a domain would actually implement
	assert.Nil(t, funcs.ConfigureDomain)

	// Check callPluginImpl handles missing function
	exerciser.doExchangeToPlugin(func(req *prototk.DomainMessage) {
		req.RequestToDomain = &prototk.DomainMessage_ConfigureDomain{}
	}, func(res *prototk.DomainMessage) {
		// Get an error back saying this request hasn't been implemented by the plugin
		assert.Regexp(t, "PD020302", *res.Header.ErrorMessage)
	})

	// Check it registered
	<-exerciser.registered

}

func TestResponseToPluginAs(t *testing.T) {
	ctx, exerciser, _, _, _, done := setupDomainTests(t)
	defer done()

	// Check responseToPluginAs handles error passthrough
	_, err := responseToPluginAs(ctx, exerciser.wrapper.Wrap(&prototk.DomainMessage{}),
		fmt.Errorf("pop"), // this will just get passed through
		func(w *string) *prototk.DomainMessage {
			return nil
		})
	assert.EqualError(t, err, "pop")

	// Check responseToPluginAs handles mismatch in response object
	_, err = responseToPluginAs(ctx, exerciser.wrapper.Wrap(&prototk.DomainMessage{
		ResponseToDomain: &prototk.DomainMessage_FindAvailableStatesRes{},
	}),
		nil,
		func(w *string) *prototk.DomainMessage {
			// it's not going to be a string
			return nil
		})
	assert.Regexp(t, "PD020301", err)
}
