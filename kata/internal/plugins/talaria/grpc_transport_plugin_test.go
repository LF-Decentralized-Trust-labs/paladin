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
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kaleido-io/paladin/kata/pkg/proto"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto/plugin"
	"github.com/stretchr/testify/assert"
)

type fakeCommsBusServer struct {
	proto.UnimplementedKataMessageServiceServer

	initializedOk  bool
	listenMessages chan destination
	recvMessages   map[destination]chan *proto.Message
	sendMessages   map[destination]chan *proto.Message
}

func (fcb *fakeCommsBusServer) Listen(lr *proto.ListenRequest, ls proto.KataMessageService_ListenServer) error {
	// On a new connection to the fake server, store the registered destination and the messages it sends
	ctx := ls.Context()

	if fcb.recvMessages[destination(lr.Destination)] == nil {
		fcb.recvMessages[destination(lr.Destination)] = make(chan *proto.Message)
	}

	if fcb.sendMessages[destination(lr.Destination)] == nil {
		fcb.sendMessages[destination(lr.Destination)] = make(chan *proto.Message)
	}

	go func() {
		fcb.listenMessages <- destination(lr.Destination)

		for {
			var msg *proto.Message
			err := ls.RecvMsg(msg)
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}

			fcb.recvMessages[destination(msg.Destination)] <- msg
		}
	}()

	messageChan := fcb.sendMessages[destination(lr.Destination)]

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messageChan:
			err := ls.Send(msg)
			if err != nil {
				return err
			}
		}
	}
}

func (fcb *fakeCommsBusServer) SubscribeToTopic(context.Context, *proto.SubscribeToTopicRequest) (*proto.SubscribeToTopicResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) SendMessage(context.Context, *proto.Message) (*proto.SendMessageResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) PublishEvent(context.Context, *proto.Event) (*proto.PublishEventResponse, error) {
	return nil, nil
}
func (fcb *fakeCommsBusServer) Status(context.Context, *proto.StatusRequest) (*proto.StatusResponse, error) {
	if fcb.initializedOk {
		return &proto.StatusResponse{
			Ok: true,
		}, nil
	}

	return nil, fmt.Errorf("bangbangbang")
}
func (fcb *fakeCommsBusServer) ListDestinations(context.Context, *proto.ListDestinationsRequest) (*proto.ListDestinationsResponse, error) {
	return nil, nil
}

type fakeExternalServer struct{}

func (fes *fakeExternalServer) QueueMessageForSend(msg *ExternalMessage) {}
func (fes *fakeExternalServer) GetMessages(dest destination) (chan *proto.Message, error) {
	return nil, nil
}
func (fes *fakeExternalServer) Shutdown() {}

func TestCreateSingleInstance(t *testing.T) {
	testDir := os.TempDir()
	testingSocketLocation := path.Join(testDir, fmt.Sprintf("%s.sock", uuid.New().String()))
	defer os.Remove(testingSocketLocation)

	commsbusListener, err := net.Listen("unix", testingSocketLocation)
	assert.NoError(t, err)
	s := grpc.NewServer()

	fakeCommsBus := &fakeCommsBusServer{
		initializedOk:  true,
		listenMessages: make(chan destination, 1),
		sendMessages:   make(map[destination]chan *proto.Message),
		recvMessages:   make(map[destination]chan *proto.Message),
	}

	proto.RegisterKataMessageServiceServer(s, fakeCommsBus)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	fakeDestinationName := "heylookatmeiamanewlistener"
	fakeInstanceName := "heylookatmeiamanewinstance"

	externalServer = &fakeExternalServer{}
	err = InitializeTransportProvider(testingSocketLocation, fakeDestinationName)
	assert.NoError(t, err)

	// Now verify that we have a message registering the listener
	newListener := <-fakeCommsBus.listenMessages
	assert.Equal(t, destination(destination(fakeDestinationName)), newListener)

	createInstanceMessage := &pb.CreateInstance{
		MessageDestination: fakeDestinationName,
		Name:               fakeInstanceName,
	}

	body, err := anypb.New(createInstanceMessage)
	assert.NoError(t, err)

	createInstancePBMessage := &proto.Message{
		Id:          uuid.New().String(),
		Destination: fakeDestinationName,
		Body:        body,
	}

	// Cool - we have a provider that's stood up and working, let's throw a create instance at it
	// and check it works properly
	fakeCommsBus.sendMessages[destination(fakeDestinationName)] <- createInstancePBMessage

	var ciWG sync.WaitGroup
	ciWG.Add(1)
	instanceMade := false
	checkInstanceMadeCtx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	go func() {
		for {
			select {
			case <-checkInstanceMadeCtx.Done():
				ciWG.Done()
				return
			default:
			}

			if provider.instances[destination(fakeInstanceName)] != nil {
				instanceMade = true
				ciWG.Done()
				return
			}
		}
	}()
	ciWG.Wait()

	assert.Equal(t, true, instanceMade)
	s.Stop()
	Shutdown()
}

func TestInitializeGRPCTransportSendsStartListeningEventToCommsBus(t *testing.T) {
	// Go through the init procedure and verify that a message comes through signalling that the plugin
	// is ready to start

	testDir := os.TempDir()
	testingSocketLocation := path.Join(testDir, "grpctransport.sock")
	defer os.Remove(testingSocketLocation)

	commsbusListener, err := net.Listen("unix", testingSocketLocation)
	assert.NoError(t, err)
	s := grpc.NewServer()

	fakeCommsBus := &fakeCommsBusServer{
		initializedOk:  true,
		listenMessages: make(chan destination, 1),
		sendMessages:   make(map[destination]chan *proto.Message),
		recvMessages:   make(map[destination]chan *proto.Message),
	}

	proto.RegisterKataMessageServiceServer(s, fakeCommsBus)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	externalServer = &fakeExternalServer{}
	err = InitializeTransportProvider(testingSocketLocation, "heylookatmeiamanewlistener")
	defer Shutdown()
	assert.NoError(t, err)
	s.Stop()
	wg.Wait()

	// Now verify that we have a message registering the listener
	assert.Equal(t, 1, len(fakeCommsBus.listenMessages))
	newListener := <-fakeCommsBus.listenMessages
	assert.Equal(t, destination("heylookatmeiamanewlistener"), newListener)
}

func TestInitializeGRPCTransportDoesNotWorkWhenServerUnavailable(t *testing.T) {
	testDir := os.TempDir()
	testingSocketLocation := path.Join(testDir, "grpctransport.sock")
	defer os.Remove(testingSocketLocation)

	commsbusListener, err := net.Listen("unix", testingSocketLocation)
	assert.NoError(t, err)
	s := grpc.NewServer()
	proto.RegisterKataMessageServiceServer(s, &fakeCommsBusServer{
		initializedOk: false,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	externalServer = &fakeExternalServer{}
	err = InitializeTransportProvider(testingSocketLocation, "something")
	defer Shutdown()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bangbangbang")
	s.GracefulStop()
	wg.Wait()
}

func TestInitializeGRPCTransportTwiceThrowsError(t *testing.T) {
	testDir := os.TempDir()
	testingSocketLocation := path.Join(testDir, "grpctransport.sock")
	defer os.Remove(testingSocketLocation)

	commsbusListener, err := net.Listen("unix", testingSocketLocation)
	assert.NoError(t, err)
	s := grpc.NewServer()
	proto.RegisterKataMessageServiceServer(s, &fakeCommsBusServer{
		recvMessages:  make(map[destination]chan *proto.Message),
		sendMessages:  make(map[destination]chan *proto.Message),
		initializedOk: true,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	externalServer = &fakeExternalServer{}
	err = InitializeTransportProvider(testingSocketLocation, "something")
	assert.NoError(t, err)
	err = InitializeTransportProvider(testingSocketLocation, "something")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")
	defer Shutdown()
	s.Stop()
	commsbusListener.Close()
	wg.Wait()
}

func TestInitializeGRPCTransportSimple(t *testing.T) {
	// Just make sure the connection is good and we get no errors during start

	testDir := os.TempDir()
	testingSocketLocation := path.Join(testDir, "grpctransport.sock")
	defer os.Remove(testingSocketLocation)

	commsbusListener, err := net.Listen("unix", testingSocketLocation)
	assert.NoError(t, err)
	s := grpc.NewServer()

	fcbs := &fakeCommsBusServer{
		recvMessages:   make(map[destination]chan *proto.Message, 1),
		sendMessages:   make(map[destination]chan *proto.Message),
		listenMessages: make(chan destination, 1),
		initializedOk:  true,
	}
	proto.RegisterKataMessageServiceServer(s, fcbs)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := s.Serve(commsbusListener)
		assert.NoError(t, err)
		wg.Done()
	}()

	externalServer = &fakeExternalServer{}
	err = InitializeTransportProvider(testingSocketLocation, "something")
	assert.NoError(t, err)

	<-fcbs.listenMessages

	Shutdown()
	s.Stop()
	wg.Wait()
}

func TestBuildInfo(t *testing.T) {
	assert.Equal(t, "github.com/kaleido-io/paladin/kata/internal/plugins/talaria/grpctransport", BuildInfo())
}
