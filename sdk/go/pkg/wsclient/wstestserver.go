// Copyright © 2023 Kaleido, Inc.
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

package wsclient

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// GenerateTLSCertificates creates a key pair for server and client auth
func GenerateTLSCertficates(t *testing.T) (publicKeyFile *os.File, privateKeyFile *os.File) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyFile, _ = os.CreateTemp("", "key.pem")
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	err := pem.Encode(privateKeyFile, privateKeyBlock)
	assert.NoError(t, err)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Unit Tests"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1000 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	assert.NoError(t, err)
	publicKeyFile, _ = os.CreateTemp("", "cert.pem")
	err = pem.Encode(publicKeyFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	assert.NoError(t, err)

	return publicKeyFile, privateKeyFile
}

// NewTestTLSWSServer creates a little test server for packages (including wsclient itself) to use in unit tests
// and secured with mTLS by passing in a key pair
func NewTestTLSWSServer(testReq func(req *http.Request), publicKeyFile *os.File, privateKeyFile *os.File) (toServer, fromServer chan string, url string, done func(), err error) {
	upgrader := &websocket.Upgrader{WriteBufferSize: 1024, ReadBufferSize: 1024}
	toServer = make(chan string, 1)
	fromServer = make(chan string, 1)
	sendDone := make(chan struct{})
	receiveDone := make(chan struct{})
	connected := false
	mu := sync.Mutex{}

	handlerFunc := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if testReq != nil {
			testReq(req)
		}
		if connected {
			// test server only handles one open connection, as it only has one set of channels
			res.WriteHeader(409)
			return
		}
		ws, _ := upgrader.Upgrade(res, req, http.Header{})
		go func() {
			defer close(receiveDone)
			for {
				_, data, err := ws.ReadMessage()
				if err != nil {
					return
				}
				toServer <- string(data)
			}
		}()
		go func() {
			defer close(sendDone)
			defer func() {
				_ = ws.Close()
			}()
			for data := range fromServer {
				_ = ws.WriteMessage(websocket.TextMessage, []byte(data))
			}
		}()
		connected = true
	})

	svr := httptest.NewUnstartedServer(handlerFunc)

	cert, err := tls.LoadX509KeyPair(publicKeyFile.Name(), privateKeyFile.Name())
	if err != nil {
		return toServer, fromServer, "", nil, err
	}
	rootCAs := x509.NewCertPool()
	caPEM, _ := os.ReadFile(publicKeyFile.Name())
	rootCAs.AppendCertsFromPEM(caPEM)
	svr.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    rootCAs,
	}
	svr.StartTLS()
	addr := svr.Listener.Addr()

	return toServer, fromServer, fmt.Sprintf("wss://%s", addr), func() {
		close(fromServer)
		svr.Close()
		if connected {
			<-sendDone
			<-receiveDone
		}
	}, nil
}

// NewTestWSServer creates a little test server for packages (including wsclient itself) to use in unit tests
func NewTestWSServer(testReq func(req *http.Request)) (toServer, fromServer chan string, url string, done func()) {
	upgrader := &websocket.Upgrader{WriteBufferSize: 1024, ReadBufferSize: 1024}
	toServer = make(chan string, 1)
	fromServer = make(chan string, 1)
	sendDone := make(chan struct{})
	receiveDone := make(chan struct{})
	connected := false
	mu := sync.Mutex{}

	svr := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if testReq != nil {
			testReq(req)
		}
		if connected {
			// test server only handles one open connection, as it only has one set of channels
			res.WriteHeader(409)
			return
		}
		ws, _ := upgrader.Upgrade(res, req, http.Header{})
		go func() {
			defer close(receiveDone)
			for {
				_, data, err := ws.ReadMessage()
				if err != nil {
					return
				}
				toServer <- string(data)
			}
		}()
		go func() {
			defer close(sendDone)
			defer func() {
				_ = ws.Close()
			}()
			for data := range fromServer {
				_ = ws.WriteMessage(websocket.TextMessage, []byte(data))
			}
		}()
		connected = true
	}))
	return toServer, fromServer, fmt.Sprintf("ws://%s", svr.Listener.Addr()), func() {
		close(fromServer)
		svr.Close()
		if connected {
			<-sendDone
			<-receiveDone
		}
	}
}
