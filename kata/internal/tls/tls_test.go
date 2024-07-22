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

package tls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

func buildSelfSignedTLSKeyPair(t *testing.T, subject pkix.Name) (string, string) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	privateKeyPEM := &strings.Builder{}
	err := pem.Encode(privateKeyPEM, privateKeyBlock)
	require.NoError(t, err)
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(100 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	require.NoError(t, err)
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	require.NoError(t, err)
	publicKeyPEM := &strings.Builder{}
	err = pem.Encode(publicKeyPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, err)
	return publicKeyPEM.String(), privateKeyPEM.String()
}

func buildSelfSignedTLSKeyPairFiles(t *testing.T, subject pkix.Name) (string, string) {
	// Create an X509 certificate pair
	privatekey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	tmpDir := t.TempDir()
	privateKeyFile, _ := os.CreateTemp(tmpDir, "key.pem")
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	err = pem.Encode(privateKeyFile, privateKeyBlock)
	assert.NoError(t, err)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(100 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	assert.NoError(t, err)
	publicKeyFile, err := os.CreateTemp(tmpDir, "cert.pem")
	assert.NoError(t, err)
	err = pem.Encode(publicKeyFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	assert.NoError(t, err)
	return publicKeyFile.Name(), privateKeyFile.Name()
}

func buildTLSListener(t *testing.T, conf *Config, tlsType TLSType) (string, func()) {

	// Create the TLS Config with the CA pool and enable Client certificate validation
	tlsConfig, err := BuildTLSConfig(context.Background(), conf, tlsType)
	assert.NoError(t, err)

	// Create a Server instance to listen on port 8443 with the TLS config
	server, err := tls.Listen("tcp4", "127.0.0.1:0", tlsConfig)
	assert.NoError(t, err)

	listenerDone := make(chan struct{})
	go func() {
		defer close(listenerDone)
		for {
			tlsConn, err := server.Accept()
			if err != nil {
				t.Logf("Server ending: %s", err)
				return
			}
			// Just read until EOF, echoing back
			for {
				oneByte := make([]byte, 1)
				_, err = tlsConn.Read(oneByte)
				if err != nil {
					t.Logf("read failed: %s", err)
					break
				}
				_, err = tlsConn.Write(oneByte)
				assert.NoError(t, err)
			}
			tlsConn.Close()
		}
	}()
	return server.Addr().String(), func() {
		err := server.Close()
		assert.NoError(t, err)
		<-listenerDone
	}

}

func TestNilIfNotEnabled(t *testing.T) {

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{}, ClientType)
	assert.NoError(t, err)
	assert.Nil(t, tlsConfig)

}

func TestTLSDefault(t *testing.T) {

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
	}, ClientType)
	assert.NoError(t, err)
	assert.NotNil(t, tlsConfig)

	assert.False(t, tlsConfig.InsecureSkipVerify)
	assert.Equal(t, tls.NoClientCert, tlsConfig.ClientAuth)

}

func TestErrInvalidCAFile(t *testing.T) {

	config.RootConfigReset()
	_, notTheCAFileTheKey := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
		CAFile:  notTheCAFileTheKey,
	}, ClientType)
	assert.Regexp(t, "PD010801", err)
}

func TestErrInvalidCA(t *testing.T) {

	config.RootConfigReset()
	_, notTheCATheKey := buildSelfSignedTLSKeyPair(t, pkix.Name{
		CommonName: "server.example.com",
	})

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
		CA:      notTheCATheKey,
	}, ClientType)
	assert.Regexp(t, "PD010801", err)
}

func TestErrInvalidKeyPairFile(t *testing.T) {

	config.RootConfigReset()
	notTheKeyFile, notTheCertFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:  true,
		KeyFile:  notTheKeyFile,
		CertFile: notTheCertFile,
	}, ClientType)
	assert.Regexp(t, "PD010802", err)

}

func TestErrInvalidKeyPair(t *testing.T) {

	config.RootConfigReset()
	notTheKey, notTheCert := buildSelfSignedTLSKeyPair(t, pkix.Name{
		CommonName: "server.example.com",
	})

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
		Key:     notTheKey,
		Cert:    notTheCert,
	}, ClientType)
	assert.Regexp(t, "PD010802", err)

}

func TestMTLSOk(t *testing.T) {
	serverPublicKey, serverKey := buildSelfSignedTLSKeyPair(t, pkix.Name{
		CommonName: "server.example.com",
	})
	clientPublicKey, clientKey := buildSelfSignedTLSKeyPair(t, pkix.Name{
		CommonName: "client.example.com",
	})

	addr, done := buildTLSListener(t, &Config{
		Enabled:    true,
		CA:         clientPublicKey,
		Cert:       serverPublicKey,
		Key:        serverKey,
		ClientAuth: true,
	}, ServerType)
	defer done()

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
		CA:      serverPublicKey,
		Cert:    clientPublicKey,
		Key:     clientKey,
	}, ClientType)
	assert.NoError(t, err)
	conn, err := tls.Dial("tcp4", addr, tlsConfig)
	assert.NoError(t, err)
	written, err := conn.Write([]byte{42})
	assert.NoError(t, err)
	assert.Equal(t, written, 1)
	readBytes := []byte{0}
	readCount, err := conn.Read(readBytes)
	assert.NoError(t, err)
	assert.Equal(t, readCount, 1)
	assert.Equal(t, []byte{42}, readBytes)
	_ = conn.Close()

}

func TestMTLSMissingClientCert(t *testing.T) {

	serverPublicKeyFile, serverKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})

	addr, done := buildTLSListener(t, &Config{
		Enabled:    true,
		CAFile:     serverPublicKeyFile,
		CertFile:   serverPublicKeyFile,
		KeyFile:    serverKeyFile,
		ClientAuth: true,
	}, ServerType)
	defer done()

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled: true,
		CAFile:  serverPublicKeyFile,
	}, ClientType)
	assert.NoError(t, err)
	conn, err := tls.Dial("tcp4", addr, tlsConfig)
	assert.NoError(t, err)
	_, _ = conn.Write([]byte{1})
	_, err = conn.Read([]byte{1})
	assert.Regexp(t, "certificate required", err)
	_ = conn.Close()

}

func TestMTLSMatchFullSubject(t *testing.T) {

	serverPublicKeyFile, serverKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})
	clientPublicKeyFile, clientKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName:         "client.example.com",
		Country:            []string{"GB"},
		Organization:       []string{"hyperledger"},
		OrganizationalUnit: []string{"firefly"},
		Province:           []string{"SomeCounty"},
		Locality:           []string{"SomeTown"},
		StreetAddress:      []string{"SomeAddress"},
		PostalCode:         []string{"AB12 3CD"},
		SerialNumber:       "12345",
	})

	addr, done := buildTLSListener(t, &Config{
		Enabled:    true,
		CAFile:     clientPublicKeyFile,
		CertFile:   serverPublicKeyFile,
		KeyFile:    serverKeyFile,
		ClientAuth: true,
		RequiredDNAttributes: map[string]interface{}{
			"cn":           `[a-z]+\.example\.com`,
			"C":            "GB",
			"O":            "hyperledger",
			"OU":           "firefly",
			"ST":           "SomeCounty",
			"L":            "SomeTown",
			"STREET":       "SomeAddress",
			"POSTALCODE":   "AB12 3CD",
			"SERIALNUMBER": "12345",
		},
	}, ServerType)
	defer done()

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:  true,
		CAFile:   serverPublicKeyFile,
		CertFile: clientPublicKeyFile,
		KeyFile:  clientKeyFile,
	}, ClientType)
	assert.NoError(t, err)
	conn, err := tls.Dial("tcp4", addr, tlsConfig)
	assert.NoError(t, err)
	written, err := conn.Write([]byte{42})
	assert.NoError(t, err)
	assert.Equal(t, written, 1)
	readBytes := []byte{0}
	readCount, err := conn.Read(readBytes)
	assert.NoError(t, err)
	assert.Equal(t, readCount, 1)
	assert.Equal(t, []byte{42}, readBytes)
	_ = conn.Close()

}

func TestMTLSMismatchSubject(t *testing.T) {

	serverPublicKeyFile, serverKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})
	clientPublicKeyFile, clientKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "wrong.example.com",
	})

	addr, done := buildTLSListener(t, &Config{
		Enabled:    true,
		CAFile:     clientPublicKeyFile,
		CertFile:   serverPublicKeyFile,
		KeyFile:    serverKeyFile,
		ClientAuth: true,
		RequiredDNAttributes: map[string]interface{}{
			"cn": `right\.example\.com`,
		},
	}, ServerType)
	defer done()

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:  true,
		CAFile:   serverPublicKeyFile,
		CertFile: clientPublicKeyFile,
		KeyFile:  clientKeyFile,
	}, ClientType)
	assert.NoError(t, err)
	conn, err := tls.Dial("tcp4", addr, tlsConfig)
	assert.NoError(t, err)
	_, _ = conn.Write([]byte{1})
	_, err = conn.Read([]byte{1})
	assert.Regexp(t, "bad certificate", err)
	_ = conn.Close()
}

func TestSubjectDNKnownAttributesAlwaysArray(t *testing.T) {

	assert.Equal(t, []string{}, SubjectDNKnownAttributes["CN"](pkix.Name{}))
	assert.Equal(t, []string{}, SubjectDNKnownAttributes["SERIALNUMBER"](pkix.Name{}))

}

func TestMTLSInvalidDNConfUnknown(t *testing.T) {

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:    true,
		ClientAuth: true,
		RequiredDNAttributes: map[string]interface{}{
			"unknown": "anything",
		},
	}, ServerType)
	assert.Regexp(t, "PD010803", err)

}

func TestMTLSInvalidDNConfBadMap(t *testing.T) {

	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:    true,
		ClientAuth: true,
		RequiredDNAttributes: map[string]interface{}{
			"cn": map[string]interface{}{
				"some": "nestedness",
			},
		},
	}, ServerType)
	assert.Regexp(t, "PD010804", err)

}

func TestMTLSInvalidDNConfBadRegexp(t *testing.T) {
	_, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:    true,
		ClientAuth: true,
		RequiredDNAttributes: map[string]interface{}{
			"cn": "((((open regexp",
		},
	}, ServerType)
	assert.Regexp(t, "PD010805", err)
}

func TestMTLSDNValidatorNotVerified(t *testing.T) {

	testValidator, err := buildDNValidator(context.Background(), map[string]interface{}{
		"cn": "test",
	})
	assert.NoError(t, err)

	err = testValidator(nil, nil)
	assert.Regexp(t, "PD010806", err)
}

func TestMTLSDNValidatorEmptyChain(t *testing.T) {

	testValidator, err := buildDNValidator(context.Background(), map[string]interface{}{
		"cn": "test",
	})
	assert.NoError(t, err)

	err = testValidator(nil, [][]*x509.Certificate{{}})
	assert.Regexp(t, "PD010806", err)

}

func TestConnectSkipVerification(t *testing.T) {

	serverPublicKeyFile, serverKeyFile := buildSelfSignedTLSKeyPairFiles(t, pkix.Name{
		CommonName: "server.example.com",
	})

	addr, done := buildTLSListener(t, &Config{
		Enabled:  true,
		CAFile:   serverPublicKeyFile,
		CertFile: serverPublicKeyFile,
		KeyFile:  serverKeyFile,
	}, ServerType)
	defer done()

	tlsConfig, err := BuildTLSConfig(context.Background(), &Config{
		Enabled:                true,
		InsecureSkipHostVerify: true,
	}, ClientType)
	assert.NoError(t, err)
	conn, err := tls.Dial("tcp4", addr, tlsConfig)
	assert.NoError(t, err)
	written, err := conn.Write([]byte{42})
	assert.NoError(t, err)
	assert.Equal(t, written, 1)
	readBytes := []byte{0}
	readCount, err := conn.Read(readBytes)
	assert.NoError(t, err)
	assert.Equal(t, readCount, 1)
	assert.Equal(t, []byte{42}, readBytes)
	_ = conn.Close()

}
