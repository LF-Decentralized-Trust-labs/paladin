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
	"context"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
)

type testController struct {
	prototk.UnimplementedPluginControllerServer
	server     *grpc.Server
	socketFile string

	fakeDomainController func(grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error
}

func newTestController(t *testing.T) (context.Context, *testController, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	tc := &testController{
		server:     grpc.NewServer(),
		socketFile: tempSocketFile(t),
	}
	prototk.RegisterPluginControllerServer(tc.server, tc)

	l, err := net.Listen("unix", tc.socketFile)
	assert.NoError(t, err)

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		_ = tc.server.Serve(l)
	}()

	return ctx, tc, func() {
		cancelCtx()
		tc.server.Stop()
		<-serverDone
	}
}

func (tc *testController) ConnectDomain(stream grpc.BidiStreamingServer[prototk.DomainMessage, prototk.DomainMessage]) error {
	return tc.fakeDomainController(stream)
}

func tempSocketFile(t *testing.T) string {
	// note socket filenames need to be <108 chars
	f, err := os.CreateTemp("", "ptk.*.sock")
	assert.NoError(t, err)
	fileName := f.Name()
	err = f.Close()
	assert.NoError(t, err)
	err = os.Remove(fileName)
	assert.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(fileName)
	})
	return fileName
}

func checkPanic() {
	panicked := recover()
	if panicked != nil {
		fmt.Fprintln(os.Stderr, (string)(debug.Stack()))
		panic(panicked)
	}

}
