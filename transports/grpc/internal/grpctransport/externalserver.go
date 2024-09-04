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
	"gopkg.in/yaml.v3"

	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/prototk"
	"github.com/kaleido-io/paladin/transports/grpc/pkg/proto"
)

type ExternalMessage struct {
	Body             string
	TransportDetails string
}

type TransportDetails struct {
	Address       string `yaml:"address"`
	CaCertificate string `yaml:"caCertificate"`
}

type ExternalServer interface {
	QueueMessageForSend(msg *proto.ExternalMessage)
	GetMessages() chan *proto.Message
	Shutdown()
}

type externalGRPCServer struct {
	proto.UnimplementedInterPaladinTransportServer

	grpcListener      net.Listener
	server            *grpc.Server
	clientCertificate *tls.Certificate
	serverCertificate *tls.Certificate
	serverCertPool    *x509.CertPool

	// TODO: We probably don't want to do this, what happens when we're not consuming messages correctly?
	recvMessages chan *prototk.TransportMessage
	sendMessages chan *ExternalMessage
	port         int
}

func NewExternalGRPCServer(ctx context.Context, port int, serverCertificate *tls.Certificate, clientCertificate *tls.Certificate) (*externalGRPCServer, error) {
	if clientCertificate == nil {
		log.L(ctx).Warnf("grpcexternal: no client certificate provided, server will be unable to do mTLS")
	}

	if serverCertificate == nil {
		log.L(ctx).Warnf("grpcexternal: no server certificate provided, server will be unable to do TLS")
	}

	server := &externalGRPCServer{
		recvMessages:      make(chan *prototk.TransportMessage, 1),
		sendMessages:      make(chan *ExternalMessage, 1),
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

func (egs *externalGRPCServer) QueueMessageForSend(msg string, transportDetails string) error {
	egs.sendMessages <- &ExternalMessage{
		Body:             msg,
		TransportDetails: transportDetails,
	}

	return nil
}

func (egs *externalGRPCServer) GetMessages() <-chan *prototk.TransportMessage {
	return egs.recvMessages
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
	proto.RegisterInterPaladinTransportServer(s, egs)

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
					ti := &TransportDetails{}
					err := yaml.Unmarshal([]byte(sendMsg.TransportDetails), ti)
					if err != nil {
						log.L(ctx).Errorf("grpctransport: could not unmarshal transport information")
						continue
					}

					// Need to get the client cert out of the message and put this in our pool
					if ok := egs.serverCertPool.AppendCertsFromPEM([]byte(ti.CaCertificate)); !ok {
						log.L(ctx).Errorf("grpctransport: could not append the client cert to the pool")
					}

					inpalMessage := &proto.InterPaladinMessage{
						Body: []byte(sendMsg.Body),
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

					client := proto.NewInterPaladinTransportClient(conn)

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

func (egs *externalGRPCServer) SendInterPaladinMessage(ctx context.Context, message *proto.InterPaladinMessage) (*proto.InterPaladinMessage, error) {
	transportedMessage := &prototk.TransportMessage{}
	err := yaml.Unmarshal(message.Body, transportedMessage)
	if err != nil {
		return nil, err
	}
	egs.recvMessages <- transportedMessage
	return nil, nil
}
