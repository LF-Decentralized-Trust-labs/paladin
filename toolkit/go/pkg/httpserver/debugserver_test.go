// Copyright © 2022 Kaleido, Inc.
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

package httpserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDebugServer(t *testing.T, conf *pldconf.HTTPServerConfig) (string, *debugServer, func()) {
	conf.Address = confutil.P("127.0.0.1")
	conf.Port = confutil.P(0)
	s, err := NewDebugServer(context.Background(), conf)
	require.NoError(t, err)
	ds := s.(*debugServer)
	err = s.Start()
	require.NoError(t, err)

	return fmt.Sprintf("http://%s", s.Addr()), ds, s.Stop
}

func TestDebugServerStackTrace(t *testing.T) {

	url, ds, done := newTestDebugServer(t, &pldconf.HTTPServerConfig{})
	defer done()

	resp, err := http.Get(fmt.Sprintf("%s/debug/pprof/goroutine?debug=2", url))
	require.NoError(t, err)

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Regexp(t, "debugserver_test.go", string(b))

	require.NotNil(t, ds.Router())

}

func TestDebugServerFail(t *testing.T) {

	_, err := NewDebugServer(context.Background(), &pldconf.HTTPServerConfig{})
	assert.Regexp(t, "PD020601", err)

}
