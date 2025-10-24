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

package componenttest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"

	"context"
	"net"
	"testing"
	"time"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/config"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/SimpleStorage.json
var simpleStorageBuildJSON []byte // From "gradle copyTestSolidityBuild"

func transactionReceiptCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has a receipt
	return func() bool {
		txFull := pldapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransactionFull", txID)
		fmt.Printf("Transaction full: %+v\n", txFull)
		require.NoError(t, err)
		require.False(t, (txFull.Receipt != nil && txFull.Receipt.Success == false), "Have transaction receipt but not successful")
		return txFull.Receipt != nil && (!isDeploy || (txFull.Receipt.ContractAddress != nil && *txFull.Receipt.ContractAddress != pldtypes.EthAddress{}))
	}
}

func transactionRevertedCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has been reverted
	return func() bool {
		txFull := pldapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransactionFull", txID)
		require.NoError(t, err)
		return txFull.Receipt != nil &&
			!txFull.Receipt.Success
	}
}

func transactionLatencyThreshold(t *testing.T) time.Duration {
	// normally we would expect a transaction to be confirmed within a couple of seconds but
	// if we are in a debug session, we want to give it much longer
	threshold := 5 * time.Second

	deadline, ok := t.Deadline()
	if !ok {
		//there was no -timeout flag, default to a long time because this is most likely a debug session
		threshold = time.Hour
	} else {
		timeRemaining := time.Until(deadline)

		//Need to leave some time to ensure that polling assertions fail before the test itself timesout
		//otherwise we don't see diagnostic info for things like GoExit called by mocks etc
		timeRemaining = timeRemaining - 100*time.Millisecond

		if timeRemaining < threshold {
			threshold = timeRemaining - 100*time.Millisecond
		}
	}
	t.Logf("Using transaction latency threshold of %v", threshold)

	return threshold
}

func testConfig(t *testing.T, enableWS bool) (pldconf.PaladinConfig, pldconf.WSClientConfig) {
	ctx := context.Background()

	var conf *pldconf.PaladinConfig
	err := config.ReadAndParseYAMLFile(ctx, "../test/config/sqlite.memory.config.yaml", &conf)
	assert.NoError(t, err)

	// For running in this unit test the dirs are different to the sample config
	// conf.DB.SQLite.DebugQueries = true
	conf.DB.SQLite.MigrationsDir = "../db/migrations/sqlite"
	// conf.DB.Postgres.DebugQueries = true
	conf.DB.Postgres.MigrationsDir = "../db/migrations/postgres"

	httpPort, err := getFreePort()
	require.NoError(t, err, "Error finding a free port for http")
	conf.GRPC.ShutdownTimeout = confutil.P("0s")
	conf.RPCServer.HTTP.ShutdownTimeout = confutil.P("0s")
	conf.RPCServer.HTTP.Port = &httpPort
	conf.RPCServer.HTTP.Address = confutil.P("127.0.0.1")

	var wsConfig pldconf.WSClientConfig
	if enableWS {
		wsPort, err := getFreePort()
		require.NoError(t, err, "Error finding a free port for ws")
		conf.RPCServer.WS.Disabled = false
		conf.RPCServer.WS.ShutdownTimeout = confutil.P("0s")
		conf.RPCServer.WS.Port = &wsPort
		conf.RPCServer.WS.Address = confutil.P("127.0.0.1")

		wsConfig.URL = fmt.Sprintf("ws://127.0.0.1:%d", wsPort)
	}

	conf.Log.Level = confutil.P("info")

	conf.TransportManagerConfig.ReliableMessageWriter.BatchMaxSize = confutil.P(1)

	conf.Wallets[0].Signer.KeyStore.Static.Keys["seed"] = pldconf.StaticKeyEntryConfig{
		Encoding: "hex",
		Inline:   pldtypes.RandHex(32),
	}

	conf.Log = pldconf.LogConfig{
		Level:  confutil.P("debug"),
		Output: confutil.P("file"),
		File: pldconf.LogFileConfig{
			Filename: confutil.P("build/testbed.component-test.log"),
		},
	}
	log.InitConfig(&conf.Log)

	return *conf, wsConfig

}

// getFreePort finds an available TCP port and returns it.
func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0") // localhost so we're not opening ports on the machine that need firewall approval
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = listener.Close()
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	return port, nil
}

func buildTestCertificate(t *testing.T, subject pkix.Name, ca *x509.Certificate, caKey *rsa.PrivateKey) (string, string) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 1024 /* smallish key to make the test faster */)
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
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:              []string{"127.0.0.1", "localhost"},
	}
	require.NoError(t, err)
	if ca == nil {
		ca = x509Template
		caKey = privatekey
		x509Template.IsCA = true
		x509Template.KeyUsage |= x509.KeyUsageCertSign
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, ca, publickey, caKey)
	require.NoError(t, err)
	publicKeyPEM := &strings.Builder{}
	err = pem.Encode(publicKeyPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, err)
	return publicKeyPEM.String(), privateKeyPEM.String()
}
