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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/kata/pkg/proto"
	
	interPaladinPB "github.com/kaleido-io/paladin/kata/pkg/proto/interpaladin"
	grpctransportpb "github.com/kaleido-io/paladin/kata/pkg/proto/grpctransport"
)

type ExternalMessage struct {
	proto.Message

	// Where are we sending the message to?
	ExternalAddress string

	// Who signed the cert of the endpoint we're talking to?
	CACertificate string
}

type ExternalServer interface {
	QueueMessageForSend(msg *proto.ExternalMessage)
	GetMessages(dest destination) chan *proto.Message
	Shutdown()
}

type externalGRPCServer struct {
	interPaladinPB.UnimplementedInterPaladinTransportServer

	grpcListener      net.Listener
	server            *grpc.Server
	clientCertificate *tls.Certificate
	serverCertificate *tls.Certificate
	serverCertPool    *x509.CertPool

	// TODO: We probably don't want to do this, what happens when we're not consuming messages correctly?
	recvMessages map[destination]chan *proto.Message
	sendMessages chan *proto.ExternalMessage
	port         int
}

func NewExternalGRPCServer(ctx context.Context, port int, bufferSize int, serverCertificate *tls.Certificate, clientCertificate *tls.Certificate) (*externalGRPCServer, error) {
	if clientCertificate == nil {
		log.L(ctx).Warnf("grpcexternal: no client certificate provided, server will be unable to do mTLS")
	}

	if serverCertificate == nil {
		log.L(ctx).Warnf("grpcexternal: no server certificate provided, server will be unable to do TLS")
	}

	server := &externalGRPCServer{
		recvMessages:      make(map[destination]chan *proto.Message, bufferSize),
		sendMessages:      make(chan *proto.ExternalMessage, bufferSize),
		port:              port,
		clientCertificate: clientCertificate,
		serverCertificate: serverCertificate,
	}

	err := server.initializeExternalListener(ctx)
	if err != nil {
		log.L(ctx).Errorf("grpcexternal: Error initializing external listener: %v", err)
		return nil, err
	}

	return server, nil
}

func (egs *externalGRPCServer) QueueMessageForSend(msg *proto.ExternalMessage) {
	egs.sendMessages <- msg
}

func (egs *externalGRPCServer) GetMessages(dest destination) chan *proto.Message {
	if egs.recvMessages[dest] == nil {
		egs.recvMessages[dest] = make(chan *proto.Message, 1) // TODO: Change this
	}

	return egs.recvMessages[dest]
}

func (egs *externalGRPCServer) Shutdown() {
	egs.server.GracefulStop()
	egs.grpcListener.Close()
}

func (egs *externalGRPCServer) initializeExternalListener(ctx context.Context) error {
	var serverTLSConfig *tls.Config
	if egs.serverCertificate != nil {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			log.L(ctx).Errorf("grpctransport: could not get system cert pool, continuing with an empty cert pool, err: %v", err)
			rootCAs = x509.NewCertPool()
		}

		// Done so we can live update the CAs we trust
		egs.serverCertPool = rootCAs
		serverTLSConfig = &tls.Config{
			RootCAs:      egs.serverCertPool,
			ClientCAs:    egs.serverCertPool,
			Certificates: []tls.Certificate{*egs.serverCertificate},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
	}

	var clientTLSConfig *tls.Config
	if egs.clientCertificate != nil {
		clientTLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*egs.clientCertificate},
			RootCAs:      egs.serverCertPool, // Client connections trust the same CAs as we accept on the server
		}
	}

	serverTLSConfig.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
		return &tls.Config{
			RootCAs:      egs.serverCertPool,
			ClientCAs:    egs.serverCertPool,
			Certificates: []tls.Certificate{*egs.serverCertificate},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}, nil
	}

	externalGRPCListener, err := net.Listen("tcp", fmt.Sprintf(":%d", egs.port))
	if err != nil {
		log.L(ctx).Errorf("grpctransport: failed to listen for external grpc connections: %v", err)
		return err
	}

	egs.grpcListener = externalGRPCListener

	var s *grpc.Server
	if serverTLSConfig != nil {
		s = grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLSConfig)))
	} else {
		s = grpc.NewServer()
	}

	egs.server = s
	interPaladinPB.RegisterInterPaladinTransportServer(s, egs)

	// Monitor new messages coming in from the network
	go func() {
		log.L(ctx).Infof("grpctransport: external gRPC endpoint listening at %v", externalGRPCListener.Addr())
		if err := s.Serve(externalGRPCListener); err != nil {
			if err == io.EOF {
				return
			}
			log.L(ctx).Errorf("failed to serve: %v", err)
		}
	}()

	// Also monitor the send queue for us to send outbound messages from
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sendMsg := <-egs.sendMessages:
				{
					// Unmarshal the transport information
					ti := &grpctransportpb.GRPCTransportInformation{}
					err := sendMsg.GetTransportInformation().UnmarshalTo(ti)
					if err != nil {
						log.L(ctx).Errorf("grpctransport: could not unmarshal transport information")
						continue
					}

					// Need to get the client cert out of the message and put this in our pool
					if ok := egs.serverCertPool.AppendCertsFromPEM([]byte(ti.CaCertificate)); !ok {
						log.L(ctx).Errorf("grpctransport: could not append the client cert to the pool")
					}

					inpalMessage := &interPaladinPB.InterPaladinMessage{
						Body: sendMsg.Body,
					}

					var conn *grpc.ClientConn
					if clientTLSConfig != nil {
						conn, err = grpc.NewClient(ti.Address, grpc.WithTransportCredentials(credentials.NewTLS(clientTLSConfig)))
						if err != nil {
							log.L(ctx).Errorf("Failed to establish a client, err: %s", err)
						}
					} else {
						conn, err = grpc.NewClient(ti.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
						if err != nil {
							log.L(ctx).Errorf("Failed to establish a client, err: %s", err)
						}
					}
					defer conn.Close()

					client := interPaladinPB.NewInterPaladinTransportClient(conn)

					_, err = client.SendInterPaladinMessage(ctx, inpalMessage)
					if err != nil {
						log.L(ctx).Errorf("error sending message: %s", err.Error())
					}

					return
				}
			}
		}
	}()

	return nil
}

func (egs *externalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *interPaladinPB.InterPaladinMessage) (*interPaladinPB.InterPaladinMessage, error) {
	recvMessage := &ExternalMessage{}
	err := message.GetBody().UnmarshalTo(recvMessage)
	if err != nil {
		return nil, err
	}

	if egs.recvMessages[destination(recvMessage.Destination)] == nil {
		egs.recvMessages[destination(recvMessage.Destination)] = make(chan *proto.Message, 1)
	}

	egs.recvMessages[destination(recvMessage.Destination)] <- &recvMessage.Message
	return nil, nil
}
