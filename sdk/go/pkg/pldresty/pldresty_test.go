// Copyright © 2025 Kaleido, Inc.
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

package pldresty

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestOK(t *testing.T) {
	c, err := New(context.Background(), &pldconf.HTTPClientConfig{
		URL:         "http://localhost:12345",
		HTTPHeaders: map[string]interface{}{"someheader": "headervalue"},
		Auth: pldconf.HTTPBasicAuthConfig{
			Username: "user",
			Password: "pass",
		},
		Retry: pldconf.HTTPRetryConfig{
			Enabled: true,
		},
	})
	require.Nil(t, err)
	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "headervalue", req.Header.Get("someheader"))
			assert.Equal(t, "Basic dXNlcjpwYXNz", req.Header.Get("Authorization"))
			return httpmock.NewStringResponder(200, `{"some": "data"}`)(req)
		})

	resp, err := c.R().Get("/test")
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, `{"some": "data"}`, resp.String())

	assert.Equal(t, 1, httpmock.GetTotalCallCount())
}

func TestRequestOKForGzip(t *testing.T) {
	c, err := New(context.Background(), &pldconf.HTTPClientConfig{
		URL:         "http://localhost:12345",
		HTTPHeaders: map[string]interface{}{"someheader": "headervalue"},
		Auth: pldconf.HTTPBasicAuthConfig{
			Username: "user",
			Password: "pass",
		},
	})
	require.Nil(t, err)
	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "headervalue", req.Header.Get("someheader"))
			assert.Equal(t, "Basic dXNlcjpwYXNz", req.Header.Get("Authorization"))
			resp := httpmock.NewStringResponse(200, `{"some": "data"}`)
			resp.Header.Set("Content-Encoding", "gzip")
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			if _, err := gz.Write([]byte(`{"some": "data"}`)); err != nil {
				return nil, err
			}
			if err := gz.Close(); err != nil {
				return nil, err
			}
			resp.Body = io.NopCloser(&b)
			return resp, nil
		})

	resp, err := c.R().Get("/test")
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode())
	assert.Equal(t, `{"some": "data"}`, resp.String())

	assert.Equal(t, 1, httpmock.GetTotalCallCount())
}

func TestRequestRetry(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "http://localhost:12345",
		Retry: pldconf.HTTPRetryConfig{
			Enabled:      true,
			InitialDelay: confutil.P("1ns"),
		},
	})
	require.Nil(t, err)

	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		httpmock.NewStringResponder(500, `{"message": "pop"}`))

	resp, err := c.R().Get("/test")
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode())
	assert.Equal(t, 6, httpmock.GetTotalCallCount())
}

func TestRequestRetryErrorStatusCodeRegex(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "http://localhost:12345",
		Retry: pldconf.HTTPRetryConfig{
			Enabled:          true,
			InitialDelay:     confutil.P("1ns"),
			ErrorStatusCodes: "(?:429|503)",
		},
	})
	require.Nil(t, err)

	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		httpmock.NewStringResponder(500, `{"message": "pop"}`))

	httpmock.RegisterResponder("GET", "http://localhost:12345/test2",
		httpmock.NewStringResponder(429, `{"message": "pop"}`))

	httpmock.RegisterResponder("GET", "http://localhost:12345/test3",
		httpmock.NewErrorResponder(errors.New("not http response")))

	resp, err := c.R().Get("/test")
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode())
	assert.Equal(t, 1, httpmock.GetTotalCallCount())

	resp, err = c.R().Get("/test2")
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode())
	assert.Equal(t, 7, httpmock.GetTotalCallCount())

	resp, err = c.R().Get("/test3")
	require.Error(t, err)
	assert.Equal(t, 0, resp.StatusCode())
	assert.Equal(t, 13, httpmock.GetTotalCallCount())
}

func TestLongResponse(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "http://localhost:12345",
	})
	require.Nil(t, err)
	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	resText := strings.Builder{}
	for i := 0; i < 512; i++ {
		resText.WriteByte(byte('a' + (i % 26)))
	}
	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		httpmock.NewStringResponder(500, resText.String()))

	resp, err := c.R().Get("/test")
	err = WrapRestErr(ctx, resp, err, pldmsgs.MsgInflightRequestCancelled)
	assert.Error(t, err)
}

func TestErrResponse(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "http://localhost:12345",
	})
	require.Nil(t, err)
	httpmock.ActivateNonDefault(c.GetClient())
	defer httpmock.DeactivateAndReset()

	resText := strings.Builder{}
	for i := 0; i < 512; i++ {
		resText.WriteByte(byte('a' + (i % 26)))
	}
	httpmock.RegisterResponder("GET", "http://localhost:12345/test",
		httpmock.NewErrorResponder(fmt.Errorf("pop")))

	resp, err := c.R().Get("/test")
	err = WrapRestErr(ctx, resp, err, pldmsgs.MsgInflightRequestCancelled)
	assert.Error(t, err)
}

func TestOnAfterResponseNil(t *testing.T) {
	OnAfterResponse(nil, nil)
}

func TestMissingCAFile(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "https://localhost:12345",
		TLS: pldconf.TLSConfig{
			Enabled: true,
			CAFile:  "non-existent.pem",
		},
	})
	assert.Regexp(t, "PD020401", err)
}

func TestMTLSClientWithServer(t *testing.T) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyFile, _ := os.CreateTemp("", "key.pem")
	defer os.Remove(privateKeyFile.Name())
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	_ = pem.Encode(privateKeyFile, privateKeyBlock)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Unit Tests"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(100 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	assert.NoError(t, err)
	publicKeyFile, _ := os.CreateTemp("", "cert.pem")
	defer os.Remove(publicKeyFile.Name())
	_ = pem.Encode(publicKeyFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	http.HandleFunc("/hello", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		_ = json.NewEncoder(res).Encode(map[string]interface{}{"hello": "world"})
	})

	// Create a CA certificate pool and add cert.pem to it
	caCert, err := os.ReadFile(publicKeyFile.Name())
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create the TLS Config with the CA pool and enable Client certificate validation
	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
	}

	// Create a Server instance to listen on port 8443 with the TLS config
	server := &http.Server{
		TLSConfig: tlsConfig,
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return
		}
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	defer ln.Close()

	go func() {
		_ = server.ServeTLS(ln, publicKeyFile.Name(), privateKeyFile.Name())
	}()

	// Use pldresty to test the mTLS client as well
	c, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: fmt.Sprintf("https://%s", ln.Addr()),
		TLS: pldconf.TLSConfig{
			Enabled:  true,
			KeyFile:  privateKeyFile.Name(),
			CertFile: publicKeyFile.Name(),
			CAFile:   publicKeyFile.Name(),
		},
	})
	require.NoError(t, err)

	httpsAddr := fmt.Sprintf("https://%s/hello", ln.Addr())
	fmt.Println(httpsAddr)
	res, err := c.R().Get(httpsAddr)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 200, res.StatusCode())
	var resBody map[string]interface{}
	err = json.Unmarshal(res.Body(), &resBody)
	require.NoError(t, err)
	assert.Equal(t, "world", resBody["hello"])
}

func TestInvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &pldconf.HTTPClientConfig{
		URL: "banana",
	})
	assert.Regexp(t, "PD020501", err)
}
