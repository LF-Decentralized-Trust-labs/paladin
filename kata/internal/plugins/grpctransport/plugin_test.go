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
package grpctransport

import (
	"net"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"google.golang.org/grpc"
	"github.com/stretchr/testify/assert"
)

type fakePluginController struct {
	prototk.UnimplementedPluginControllerServer

	recvMessages chan *prototk.TransportMessage
	sendMessages chan *prototk.TransportMessage
}

func (fpc *fakePluginController) ConnectTransport(stream grpc.BidiStreamingServer[prototk.TransportMessage, prototk.TransportMessage]) error {
	ctx := stream.Context()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-fpc.sendMessages:
				err := stream.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	for {
		inboundMessage, err := stream.Recv()
		if err != nil {
			return err
		}

		fpc.recvMessages <- inboundMessage
	}
}

func newFakePluginController(location string) (*fakePluginController, error) {
	listener, err := net.Listen("unix", location)
	if err != nil {
		return nil, err
	}

	fpc := &fakePluginController{
		recvMessages: make(chan *prototk.TransportMessage),
		sendMessages: make(chan *prototk.TransportMessage),
	}

	s := grpc.NewServer()
	prototk.RegisterPluginControllerServer(s, fpc)

	go func() {
		err := s.Serve(listener)
		if err != nil {
			return
		}
	}()

	return fpc, nil
}

func tempUDS(t *testing.T) string {
	// Not safe to use t.TempDir() as it generates too long paths including the test name
	f, err := os.CreateTemp("", "ut_*.sock")
	assert.NoError(t, err)
	_ = f.Close()
	allocatedUDSName := f.Name()
	err = os.Remove(allocatedUDSName)
	assert.NoError(t, err)
	t.Cleanup(func() {
		err := os.Remove(allocatedUDSName)
		assert.True(t, err == nil || os.IsNotExist(err))
	})
	return allocatedUDSName
}

func setupPluginForTest(location string) (*GRPCTransport, error) {
	transport, err := NewGRPCTransport(uuid.NewString(), fmt.Sprintf("unix://%s", location))
	if err != nil {
		return nil, err
	}

	return transport, nil
}

func TestNewGRPCTransport(t *testing.T) {
	transport, err := NewGRPCTransport(uuid.NewString(), "unix:///tmp/something")
	assert.Nil(t, err)
	assert.NotNil(t, transport)
}

func TestInitDoesNotConnectToBadLocation(t *testing.T) {
	transport, err := NewGRPCTransport(uuid.NewString(), "unix:///obviously/wrong/location")
	assert.Nil(t, err)
	assert.NotNil(t, transport)

	err = transport.Init()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}

func TestCannotConnectToThePluginController(t *testing.T) {
	testingLocation := tempUDS(t)

	transport, err := setupPluginForTest(testingLocation)
	assert.Nil(t, err)

	err = transport.Init()
	assert.Contains(t, err.Error(), "Error while dialing")
}

func TestInitializeWorksWhenPluginControllerIsActive(t *testing.T) {
	testingLocation := tempUDS(t)

	transport, err := setupPluginForTest(testingLocation)
	defer func() {
		err = transport.Shutdown()
		assert.Nil(t, err)
	}()
	assert.Nil(t, err)
	transport.externalServer = &externalGRPCServer{}

	_, err = newFakePluginController(testingLocation)
	assert.Nil(t, err)

	err = transport.Init()
	assert.Nil(t, err)

	transport.InitializeExternalListener()
}

func TestHandleConfigureTransports(t *testing.T) {
	testingLocation := tempUDS(t)

	transport, err := setupPluginForTest(testingLocation)
	defer func() {
		err = transport.Shutdown()
		assert.Nil(t, err)
	}()
	assert.Nil(t, err)
	transport.externalServer = &externalGRPCServer{}

	fpc, err := newFakePluginController(testingLocation)
	assert.Nil(t, err)

	err = transport.Init()
	assert.Nil(t, err)

	config := `serverCertificate: somecert
clientCertificate: somecert
externalPort: 8080`

	fpc.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_ConfigureTransport{
			ConfigureTransport: &prototk.ConfigureTransportRequest{
				ConfigYaml: config,
			},
		},
	}

	<-fpc.recvMessages

	assert.Equal(t, 8080, transport.Config.ExternalPort)
}

func TestHandleInitCall(t *testing.T) {
	testingLocation := tempUDS(t)

	transport, err := setupPluginForTest(testingLocation)
	defer func() {
		err = transport.Shutdown()
		assert.Nil(t, err)
	}()
	assert.Nil(t, err)
	transport.externalServer = &externalGRPCServer{}

	fpc, err := newFakePluginController(testingLocation)
	assert.Nil(t, err)

	err = transport.Init()
	assert.Nil(t, err)

	fpc.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_InitTransport{
			InitTransport: &prototk.InitTransportRequest{},
		},
	}

	<-fpc.recvMessages
}

func TestHandleSendMessages(t *testing.T) {
	testingLocation := tempUDS(t)

	transport, err := setupPluginForTest(testingLocation)
	defer func() {
		err = transport.Shutdown()
		assert.Nil(t, err)
	}()
	assert.Nil(t, err)
	transport.externalServer = &externalGRPCServer{
		sendMessages: make(chan *ExternalMessage, 1),
	}

	fpc, err := newFakePluginController(testingLocation)
	assert.Nil(t, err)

	err = transport.Init()
	assert.Nil(t, err)

	fpc.sendMessages <- &prototk.TransportMessage{
		RequestToTransport: &prototk.TransportMessage_SendMessage{
			SendMessage: &prototk.SendMessageRequest{
				Body: "something",
				TransportDetails: "somethingelse",
			},
		},
	}

	<-fpc.recvMessages
	<-transport.externalServer.sendMessages
}
