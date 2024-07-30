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

package plugins

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-common/pkg/log"
	interPaladinProto "github.com/kaleido-io/talaria/pkg/plugins/proto"
	pluginInterfaceProto "github.com/kaleido-io/talaria/pkg/talaria/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

/*
	This is the gRPC plugin that Talaria will talk to in order to send comms to other paladin
	nodes in the network. In theory, there's little stopping us connecting the gRPC transport
	layer at Talaria directly to another Talaria, but a dedicated plugin here proves that the
	plugin architecture not only works, but is in use.

	Here we implement 2 comms flows:

	1. gRPC to boundary Talaria layer using unix domain sockets
	2. gRPC over TLS to another gRPC residing on another Paladin node (potentially)

	It's important to note that it's possible for a transacting entity to need to send comms
	to another transacting entity that is actually on the same node, if that's the case then
	we still treat it as an outbound connection, but the actual call is to loopback.
*/

type GRPCRoutingInformation struct {
	// In theory this is an opaque object in the Registry that only this plugin knows how to
	// use and decode, for gRPC the amount of information we need is quite minimal though
	Address       string `json:"address"`
	CACertificate string `json:"caCertificate"`
}

type RawPemCertificate []byte

type GRPCTransportPlugin struct {
	interPaladinProto.UnimplementedInterPaladinTransportServer
	pluginInterfaceProto.UnimplementedPluginInterfaceServer

	SocketName        string
	port              int
	messages          chan []byte
	clientCertificate tls.Certificate
	caPool            *x509.CertPool
	caPoolLock        sync.Mutex

	pluginListener       net.Listener
	interPaladinListener net.Listener
}

// --------------------------------------------------------------------------------------------------------- Inter-Paladin Server

func (gtp *GRPCTransportPlugin) generateTLSConfig() *tls.Config {
	return &tls.Config{
		RootCAs:      gtp.caPool,
		ClientCAs:    gtp.caPool,
		Certificates: []tls.Certificate{gtp.clientCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
}

func (gtp *GRPCTransportPlugin) startInterPaladinMessageServer(ctx context.Context) {
	log.L(ctx).Debugf("initialising connection for inbound gRPC connections %s\n", gtp.SocketName)
	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", gtp.port))
	if err != nil {
		log.L(ctx).Errorf("failed to listen for grpc connections: %v", err)
	}

	gtp.interPaladinListener = grpcLis

	tlsConfig := gtp.generateTLSConfig()
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	interPaladinProto.RegisterInterPaladinTransportServer(s, gtp)

	go func() {
		<-ctx.Done()
		s.Stop()
		grpcLis.Close()
	}()

	go func() {
		log.L(ctx).Debugf("grpc server listening at %v", grpcLis.Addr())
		if err := s.Serve(grpcLis); err != nil {
			log.L(ctx).Errorf("failed to serve: %v", err)
		}
	}()
}

func (gtp *GRPCTransportPlugin) SendInterPaladinMessage(ctx context.Context, in *interPaladinProto.InterPaladinMessage) (*interPaladinProto.Empty, error) {
	log.L(ctx).Tracef("Got an external message")
	gtp.messages <- in.Payload
	return &interPaladinProto.Empty{}, nil
}

// --------------------------------------------------------------------------------------------------------- Plugin Server

func (gtp *GRPCTransportPlugin) startPluginServer(ctx context.Context) {
	log.L(ctx).Debugf("initialising connection to local socket %s\n", gtp.SocketName)
	lis, err := net.Listen("unix", gtp.SocketName)
	if err != nil {
		log.L(ctx).Errorf("failed to listen: %v", err)
	}

	gtp.pluginListener = lis
	s := grpc.NewServer()
	pluginInterfaceProto.RegisterPluginInterfaceServer(s, gtp)

	go func() {
		<-ctx.Done()
		s.Stop()
		lis.Close()
	}()

	go func() {
		log.L(ctx).Debugf("server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.L(ctx).Errorf("failed to serve: %v", err)
		}
	}()
}

func (gtp *GRPCTransportPlugin) PluginMessageFlow(server pluginInterfaceProto.PluginInterface_PluginMessageFlowServer) error {
	ctx := server.Context()

	go func(){
		for {
			select {
			case <- ctx.Done():
				return
			case collectedMessage := <-gtp.messages:
				{
					if err := server.Send(&pluginInterfaceProto.PaladinMessage{
						Payload: collectedMessage,
					}); err != nil {
						log.L(ctx).Errorf("send error %v", err)
					}
				}
			}
		}
	}()

	for {
		pluginReq, err := server.Recv()
		if err == io.EOF {
			log.L(ctx).Debugf("Shutting down Plugin listener")
			return nil
		}
		if err != nil {
			log.L(ctx).Errorf("receive error %v", err)
			continue
		}

		routingInfo := &GRPCRoutingInformation{}
		err = json.Unmarshal([]byte(pluginReq.RoutingInformation), routingInfo)
		if err != nil {
			log.L(ctx).Errorf("Could not unmarshal routing information, err: %v", err)
			return err
		}

		outboundCertPool := x509.NewCertPool()
		ok := outboundCertPool.AppendCertsFromPEM([]byte(routingInfo.CACertificate))
		if !ok {
			log.L(ctx).Errorf("Unable to append CA to the pool for outbound client connection")
			continue
		}

		outboundTLSConfig := &tls.Config{
			Certificates: []tls.Certificate{gtp.clientCertificate},
			RootCAs: outboundCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
		}

		conn, err := grpc.NewClient(routingInfo.Address, grpc.WithTransportCredentials(credentials.NewTLS(outboundTLSConfig)))
		if err != nil {
			log.L(ctx).Errorf("Failed to establish a client, err: %s", err)
		}
		defer conn.Close()

		client := interPaladinProto.NewInterPaladinTransportClient(conn)

		_, err = client.SendInterPaladinMessage(ctx, &interPaladinProto.InterPaladinMessage{
			Payload: pluginReq.Payload,
		})
		if err != nil {
			log.L(ctx).Errorf("error sending message through gRPC: %v", err)
		}
	}		
}

// Actually unlikely to be needed
func (gtp *GRPCTransportPlugin) Status(ctx context.Context, _ *pluginInterfaceProto.StatusRequest) (*pluginInterfaceProto.PluginStatus, error) {
	return &pluginInterfaceProto.PluginStatus{
		Ok: true,
	}, nil
}

// --------------------------------------------------------------------------------------------------------------------------

func (gtp *GRPCTransportPlugin) Start(ctx context.Context) {
	gtp.startInterPaladinMessageServer(ctx)
	gtp.startPluginServer(ctx)
}

func (gtp *GRPCTransportPlugin) AddNewKnownPeer(cert RawPemCertificate) (ok bool) {
	// The reason we have to lock is because under the covers
	// the AppendCertsFromPem adds to an array of lazy certs
	// that get loaded and checked when a certificate is validated
	gtp.caPoolLock.Lock()
	defer gtp.caPoolLock.Unlock()
	return gtp.caPool.AppendCertsFromPEM(cert)
}

// TODO: Rip all of this out and replace it with whatever registration framework we get with kata
func (gtp *GRPCTransportPlugin) GetRegistration() PluginRegistration {
	return PluginRegistration{
		Name:           "grpc-transport-plugin",
		SocketLocation: gtp.SocketName,
	}
}

// TODO: Config
func NewGRPCTransportPlugin(ctx context.Context, port int, clientCertificate tls.Certificate) *GRPCTransportPlugin {
	var certPool *x509.CertPool
	var err error
	certPool, err = x509.SystemCertPool()
	if err != nil {
		log.L(ctx).Warnf("Unable to get the default system cert pool, continuing with an empty cert pool.")
		certPool = x509.NewCertPool()
	}

	return &GRPCTransportPlugin{
		port:              port,
		SocketName:        fmt.Sprintf("/tmp/%s.sock", uuid.New().String()),
		clientCertificate: clientCertificate,
		caPool:            certPool,
		// Buffer size needs to be configurable
		messages: make(chan []byte, 10),
	}
}
