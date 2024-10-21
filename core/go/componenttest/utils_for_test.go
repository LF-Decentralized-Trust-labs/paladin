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
	"encoding/json"
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
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/plugintk"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/transports/grpc/pkg/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed abis/SimpleStorage.json
var simpleStorageBuildJSON []byte // From "gradle copyTestSolidityBuild"

func transactionReceiptCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has a receipt
	return func() bool {
		txFull := pldapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransaction", txID, true)

		require.NoError(t, err)
		require.False(t, (txFull.Receipt != nil && txFull.Receipt.Success == false), "Have transaction receipt but not successful")
		return txFull.Receipt != nil && (!isDeploy || (txFull.Receipt.ContractAddress != nil && *txFull.Receipt.ContractAddress != tktypes.EthAddress{}))
	}
}

func transactionRevertedCondition(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client) func() bool {
	//for the given transaction ID, return a function that can be used in an assert.Eventually to check if the transaction has been reverted
	return func() bool {
		txFull := pldapi.TransactionFull{}
		err := rpcClient.CallRPC(ctx, &txFull, "ptx_getTransaction", txID, true)
		require.NoError(t, err)
		return txFull.Receipt != nil &&
			!txFull.Receipt.Success
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
	name                   string
	conf                   *pldconf.PaladinConfig
	ctx                    context.Context
	client                 rpcclient.Client
	resolveEthereumAddress func(identity string) string
}

func deployDomainRegistry(t *testing.T) *tktypes.EthAddress {
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
	err = cmTmp.StartManagers()
	require.NoError(t, err)
	err = cmTmp.CompleteStart()
	require.NoError(t, err)
	domainRegistryAddress := domains.DeploySmartContract(t, cmTmp.TxManager())

	cmTmp.Stop()
	return domainRegistryAddress

}

type nodeConfiguration struct {
	address string
	port    int
	cert    string
	key     string
	name    string
}

func newNodeConfiguration(t *testing.T, nodeName string) *nodeConfiguration {
	port, err := getFreePort()
	require.NoError(t, err)
	cert, key := buildTestCertificate(t, pkix.Name{CommonName: nodeName}, nil, nil)
	return &nodeConfiguration{
		address: "localhost",
		port:    port,
		cert:    cert,
		key:     key,
		name:    nodeName,
	}
}

func newInstanceForComponentTesting(t *testing.T, domainRegistryAddress *tktypes.EthAddress, binding *nodeConfiguration, peerNodes []*nodeConfiguration, domainConfig *domains.SimpleDomainConfig) *componentTestInstance {
	if binding == nil {
		binding = newNodeConfiguration(t, "default")
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
		name:       binding.name,
		conf:       &conf,
	}
	i.ctx = log.WithLogField(context.Background(), "node-name", binding.name)

	i.conf.Log.Level = confutil.P("info")
	i.conf.BlockIndexer.FromBlock = json.RawMessage(`"latest"`)
	i.conf.DomainManagerConfig.Domains = make(map[string]*pldconf.DomainConfig, 1)
	if domainConfig == nil {
		domainConfig = &domains.SimpleDomainConfig{
			SubmitMode: domains.ENDORSER_SUBMISSION,
		}
	}
	i.conf.DomainManagerConfig.Domains["domain1"] = &pldconf.DomainConfig{
		AllowSigning: true,
		Plugin: pldconf.PluginConfig{
			Type:    string(tktypes.LibraryTypeCShared),
			Library: "loaded/via/unit/test/loader",
		},
		Config:          map[string]any{"submitMode": domainConfig.SubmitMode},
		RegistryAddress: domainRegistryAddress.String(),
	}

	i.conf.NodeName = binding.name
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
		nodesConfig[peerNode.name] = &static.StaticEntry{
			Properties: map[string]tktypes.RawJSON{
				"transport.grpc": tktypes.JSONString(
					grpc.PublishedTransportDetails{
						Endpoint: fmt.Sprintf("dns:///%s:%d", peerNode.address, peerNode.port),
						Issuers:  peerNode.cert,
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

	//uncomment for debugging
	//i.conf.DB.SQLite.DSN = "./sql." + i.name + ".db"
	i.conf.Log.Level = confutil.P("debug")

	var pl plugins.UnitTestPluginLoader

	cm := componentmgr.NewComponentManager(i.ctx, i.grpcTarget, uuid.New(), i.conf)
	// Start it up
	err = cm.Init()
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

	client, err := rpcclient.NewHTTPClient(log.WithLogField(context.Background(), "client-for", binding.name), &pldconf.HTTPClientConfig{URL: "http://localhost:" + strconv.Itoa(*i.conf.RPCServer.HTTP.Port)})
	require.NoError(t, err)
	i.client = client

	i.resolveEthereumAddress = func(identity string) string {
		idPart, err := tktypes.PrivateIdentityLocator(identity).Identity(context.Background())
		require.NoError(t, err)
		addr, err := cm.KeyManager().ResolveEthAddressNewDatabaseTX(i.ctx, idPart)
		require.NoError(t, err)
		return addr.String()
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
	conf.GRPC.ShutdownTimeout = confutil.P("0s")
	conf.RPCServer.HTTP.ShutdownTimeout = confutil.P("0s")
	conf.RPCServer.HTTP.Port = &port
	conf.RPCServer.HTTP.Address = confutil.P("127.0.0.1")
	conf.RPCServer.WS.ShutdownTimeout = confutil.P("0s")
	conf.RPCServer.WS.Disabled = true
	conf.Log.Level = confutil.P("info")

	conf.Wallets[0].Signer.KeyStore.Static.Keys["seed"] = pldconf.StaticKeyEntryConfig{
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

type partyForTesting struct {
	identity              string // identity used to resolve the verifier on its local node
	identityLocator       string // fully qualified locator for the identity that can be used on other nodes
	instance              *componentTestInstance
	nodeConfig            *nodeConfiguration
	peers                 []*nodeConfiguration
	domainRegistryAddress *tktypes.EthAddress
	client                rpcclient.Client //TODO swap out for pldclient.PaladinClient
}

func newPartyForTesting(t *testing.T, name string, domainRegistryAddress *tktypes.EthAddress) *partyForTesting {
	nodeName := name + "Node"
	party := &partyForTesting{
		peers:                 make([]*nodeConfiguration, 0),
		domainRegistryAddress: domainRegistryAddress,
		identity:              fmt.Sprintf("wallets.org1.%s", name),
		identityLocator:       fmt.Sprintf("wallets.org1.%s@%s", name, nodeName),
	}

	party.nodeConfig = newNodeConfiguration(t, nodeName)
	return party
}

func (p *partyForTesting) peer(peers ...*nodeConfiguration) {
	p.peers = append(p.peers, peers...)
}

func (p *partyForTesting) start(t *testing.T, domainConfig domains.SimpleDomainConfig) {
	p.instance = newInstanceForComponentTesting(t, p.domainRegistryAddress, p.nodeConfig, p.peers, &domainConfig)
	p.client = p.instance.client

}

func (p *partyForTesting) deploySimpleDomainInstanceContract(t *testing.T, domainRegistryAddress *tktypes.EthAddress, endorsementMode string, constructorParameters *domains.ConstructorParameters) *tktypes.EthAddress {

	var dplyTxID uuid.UUID

	err := p.client.CallRPC(context.Background(), &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(endorsementMode),
		Transaction: pldapi.Transaction{
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: "domain1",
			From:   p.identity,
			Data:   tktypes.JSONString(constructorParameters),
		},
	})
	require.NoError(t, err)
	assert.Eventually(t,
		transactionReceiptCondition(t, context.Background(), dplyTxID, p.client, true),
		transactionLatencyThreshold(t)+5*time.Second, //TODO deploy transaction seems to take longer than expected
		100*time.Millisecond,
		"Deploy transaction did not receive a receipt",
	)

	var dplyTxFull pldapi.TransactionFull
	err = p.client.CallRPC(context.Background(), &dplyTxFull, "ptx_getTransaction", dplyTxID, true)
	require.NoError(t, err)
	require.NotNil(t, dplyTxFull.Receipt)
	require.True(t, dplyTxFull.Receipt.Success)
	require.NotNil(t, dplyTxFull.Receipt.ContractAddress)
	return dplyTxFull.Receipt.ContractAddress
}
