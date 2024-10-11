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
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/core/componenttest/domains"
	"github.com/kaleido-io/paladin/core/internal/componentmgr"
	"github.com/kaleido-io/paladin/core/internal/plugins"
	"github.com/kaleido-io/paladin/registries/static/pkg/static"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
	"github.com/kaleido-io/paladin/transports/grpc/pkg/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/SimpleStorage.json
var simpleStorageBuildJSON []byte // From "gradle copyTestSolidityBuild"

func transactionReceiptCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has a receipt
	return func() bool {
		txFull := ptxapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransaction", txID, true)
		require.NoError(t, err)
		return txFull.Receipt != nil && (!isDeploy || txFull.Receipt.ContractAddress != nil)
	}

}

func transactionLatencyThreshold(t *testing.T) time.Duration {
	// normally we would expect a transaction to be confirmed within a couple of seconds but
	// if we are in a debug session, we want to give it much longer
	threshold := 2 * time.Second

	deadline, ok := t.Deadline()
	if !ok {
		//there was no -timeout flag, default to a long time becuase this is most likely a debug session
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

type componentTestInstance struct {
	grpcTarget             string
	id                     uuid.UUID
	conf                   *pldconf.PaladinConfig
	ctx                    context.Context
	client                 rpcclient.Client
	resolveEthereumAddress func(identity string) string
}

func deplyDomainRegistry(t *testing.T) *tktypes.EthAddress {
	// We need an engine so that we can deploy the base ledger contract for the domain
	//Actually, we only need a bare bones engine that is capable of deploying the base ledger contracts
	// could make do with assembling some core components like key manager, eth client factory, block indexer, persistence and any other dependencies they pull in
	// but is easier to just create a throwaway component manager with no domains
	tmpConf := testConfig(t)
	// wouldn't need to do this if we just created the core coponents directly
	f, err := os.CreateTemp("", "component-test.*.sock")
	require.NoError(t, err)

	grpcTarget := f.Name()

	err = f.Close()
	require.NoError(t, err)

	err = os.Remove(grpcTarget)
	require.NoError(t, err)

	cmTmp := componentmgr.NewComponentManager(context.Background(), grpcTarget, uuid.New(), &tmpConf)
	err = cmTmp.Init()
	require.NoError(t, err)
	err = cmTmp.StartComponents()
	require.NoError(t, err)
	domainRegistryAddress := domains.DeploySmartContract(t, cmTmp.BlockIndexer(), cmTmp.EthClientFactory())

	cmTmp.Stop()
	return domainRegistryAddress

}

type nodeConfiguration struct {
	identity uuid.UUID
	address  string
	port     int
	cert     string
	key      string
}

func newNodeConfiguration(t *testing.T) *nodeConfiguration {
	identity := uuid.New()
	port, err := getFreePort()
	require.NoError(t, err)
	cert, key := buildTestCertificate(t, pkix.Name{CommonName: identity.String()}, nil, nil)
	return &nodeConfiguration{
		identity: identity,
		address:  "localhost",
		port:     port,
		cert:     cert,
		key:      key,
	}
}

func newInstanceForComponentTesting(t *testing.T, domainRegistryAddress *tktypes.EthAddress, binding *nodeConfiguration, peerNodes []*nodeConfiguration) *componentTestInstance {
	if binding == nil {
		binding = newNodeConfiguration(t)
	}
	f, err := os.CreateTemp("", "component-test.*.sock")
	require.NoError(t, err)

	grpcTarget := f.Name()

	err = f.Close()
	require.NoError(t, err)

	err = os.Remove(grpcTarget)
	require.NoError(t, err)

	conf := testConfig(t)
	i := &componentTestInstance{
		grpcTarget: grpcTarget,
		id:         binding.identity,
		conf:       &conf,
	}
	i.ctx = log.WithLogField(context.Background(), "instance", binding.identity.String())

	i.conf.Log.Level = confutil.P("info")
	i.conf.DomainManagerConfig.Domains = make(map[string]*pldconf.DomainConfig, 1)
	i.conf.DomainManagerConfig.Domains["domain1"] = &pldconf.DomainConfig{
		Plugin: pldconf.PluginConfig{
			Type:    string(tktypes.LibraryTypeCShared),
			Library: "loaded/via/unit/test/loader",
		},
		Config:          map[string]any{"some": "config"},
		RegistryAddress: domainRegistryAddress.String(),
	}

	i.conf.NodeName = binding.identity.String()
	i.conf.Transports = map[string]*pldconf.TransportConfig{
		"grpc": {
			Plugin: pldconf.PluginConfig{
				Type:    string(tktypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config: map[string]any{
				"address": "localhost",
				"port":    binding.port,
				"tls": pldconf.TLSConfig{
					Enabled: true,
					Cert:    binding.cert,
					Key:     binding.key,
					//InsecureSkipHostVerify: true,
				},
				"directCertVerification": true,
			},
		},
	}

	nodesConfig := make(map[string]*static.StaticEntry)
	for _, peerNode := range peerNodes {
		nodesConfig[peerNode.identity.String()] = &static.StaticEntry{
			Properties: map[string]tktypes.RawJSON{
				"transport.grpc": tktypes.JSONString(
					grpc.PublishedTransportDetails{
						Endpoint: fmt.Sprintf("dns:///%s:%d", peerNodes[0].address, peerNodes[0].port),
						Issuers:  peerNodes[0].cert,
					},
				),
			},
		}
	}

	i.conf.Registries = map[string]*pldconf.RegistryConfig{
		"registry1": {
			Plugin: pldconf.PluginConfig{
				Type:    string(tktypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config: map[string]any{
				"entries": nodesConfig,
			},
		},
	}

	var pl plugins.UnitTestPluginLoader

	cm := componentmgr.NewComponentManager(i.ctx, i.grpcTarget, i.id, i.conf)
	// Start it up
	err = cm.Init()
	require.NoError(t, err)

	err = cm.StartComponents()
	require.NoError(t, err)

	err = cm.StartManagers()
	require.NoError(t, err)

	loaderMap := map[string]plugintk.Plugin{
		"domain1":   domains.SimpleTokenDomain(t, i.ctx),
		"grpc":      grpc.NewPlugin(i.ctx),
		"registry1": static.NewPlugin(i.ctx),
	}
	pc := cm.PluginManager()
	pl, err = plugins.NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.LoaderID().String(), loaderMap)
	require.NoError(t, err)
	go pl.Run()

	err = cm.CompleteStart()
	require.NoError(t, err)

	t.Cleanup(func() {
		pl.Stop()
		cm.Stop()
	})

	client, err := rpcclient.NewHTTPClient(log.WithLogField(context.Background(), "client-for", binding.identity.String()), &pldconf.HTTPClientConfig{URL: "http://localhost:" + strconv.Itoa(*i.conf.RPCServer.HTTP.Port)})
	require.NoError(t, err)
	i.client = client

	i.resolveEthereumAddress = func(identity string) string {
		_, address, err := cm.KeyManager().ResolveKey(i.ctx, identity, algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
		require.NoError(t, err)
		return address
	}

	return i

}

func testConfig(t *testing.T) pldconf.PaladinConfig {
	ctx := context.Background()

	var conf *pldconf.PaladinConfig
	err := pldconf.ReadAndParseYAMLFile(ctx, "../test/config/sqlite.memory.config.yaml", &conf)
	assert.NoError(t, err)

	// For running in this unit test the dirs are different to the sample config
	conf.DB.SQLite.MigrationsDir = "../db/migrations/sqlite"
	conf.DB.Postgres.MigrationsDir = "../db/migrations/postgres"

	port, err := getFreePort()
	require.NoError(t, err, "Error finding a free port")
	conf.RPCServer.HTTP.Port = &port
	conf.RPCServer.HTTP.Address = confutil.P("127.0.0.1")
	conf.Log.Level = confutil.P("info")

	conf.Signer.KeyStore.Static.Keys["seed"] = pldconf.StaticKeyEntryConfig{
		Encoding: "hex",
		Inline:   tktypes.RandHex(32),
	}

	return *conf

}

// getFreePort finds an available TCP port and returns it.
func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0") // localhost so we're not opening ports on the machine that need firewall approval
	if err != nil {
		return 0, err
	}
	defer listener.Close()

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
