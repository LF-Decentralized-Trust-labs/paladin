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

package testutils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
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

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/componentmgr"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/plugins"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/noderuntests/pkg/domains"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/config"
	"github.com/LF-Decentralized-Trust-labs/paladin/registries/static/pkg/static"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/transports/grpc/pkg/grpc"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ComponentTestInstance interface {
	GetName() string
	GetClient() rpcclient.Client
	GetWSConfig() *pldconf.WSClientConfig
	ResolveEthereumAddress(identity string) string
	GetComponentManager() componentmgr.ComponentManager
	GetPluginManager() plugins.UnitTestPluginLoader
}

type componentTestInstance struct {
	grpcTarget             string
	name                   string
	conf                   *pldconf.PaladinConfig
	ctx                    context.Context
	client                 rpcclient.Client
	resolveEthereumAddress func(identity string) string
	cm                     componentmgr.ComponentManager
	pluginManager          plugins.UnitTestPluginLoader
	wsConfig               *pldconf.WSClientConfig
}

type nodeConfiguration struct {
	address         string
	port            int
	cert            string
	key             string
	name            string
	sequencerConfig *pldconf.SequencerConfig
}

func NewNodeConfiguration(t *testing.T, nodeName string) *nodeConfiguration {
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

func (testutils *componentTestInstance) GetName() string {
	return testutils.name
}

func (testutils *componentTestInstance) GetClient() rpcclient.Client {
	return testutils.client
}

func (testutils *componentTestInstance) GetWSConfig() *pldconf.WSClientConfig {
	return testutils.wsConfig
}

func (testutils *componentTestInstance) ResolveEthereumAddress(identity string) string {
	return testutils.resolveEthereumAddress(identity)
}

func (testutils *componentTestInstance) GetComponentManager() componentmgr.ComponentManager {
	return testutils.cm
}

func (testutils *componentTestInstance) GetPluginManager() plugins.UnitTestPluginLoader {
	return testutils.pluginManager
}

func NewInstanceForTesting(t *testing.T, domainRegistryAddress *pldtypes.EthAddress, bindingConfig interface{}, peerNodes []interface{}, domainConfig interface{}, enableWS bool, configPath string, manualTestCleanup bool) ComponentTestInstance {
	var binding *nodeConfiguration
	if bindingConfig == nil {
		binding = NewNodeConfiguration(t, "default")
	} else {
		binding = bindingConfig.(*nodeConfiguration)
	}
	f, err := os.CreateTemp("", "component-test.*.sock")
	require.NoError(t, err)

	grpcTarget := f.Name()

	err = f.Close()
	require.NoError(t, err)

	err = os.Remove(grpcTarget)
	require.NoError(t, err)

	conf, wsConfig := testConfig(t, enableWS, configPath)
	i := &componentTestInstance{
		grpcTarget: grpcTarget,
		name:       binding.name,
		conf:       &conf,
		wsConfig:   &wsConfig,
	}
	i.ctx = log.WithLogField(context.Background(), "node-name", binding.name)

	if binding.sequencerConfig != nil {
		i.conf.SequencerManager.RequestTimeout = binding.sequencerConfig.RequestTimeout
		i.conf.SequencerManager.AssembleTimeout = binding.sequencerConfig.AssembleTimeout
		i.conf.SequencerManager.BlockHeightTolerance = binding.sequencerConfig.BlockHeightTolerance
		i.conf.SequencerManager.ClosingGracePeriod = binding.sequencerConfig.ClosingGracePeriod
	}

	i.conf.BlockIndexer.FromBlock = json.RawMessage(`"latest"`)
	i.conf.DomainManagerConfig.Domains = make(map[string]*pldconf.DomainConfig, 1)
	if domainConfig == nil {
		domainConfig = &domains.SimpleDomainConfig{
			SubmitMode: domains.ENDORSER_SUBMISSION,
		}
	}

	switch domainConfig := domainConfig.(type) {
	case *domains.SimpleDomainConfig:
		i.conf.DomainManagerConfig.Domains["domain1"] = &pldconf.DomainConfig{
			AllowSigning: true,
			Plugin: pldconf.PluginConfig{
				Type:    string(pldtypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config:          map[string]any{"submitMode": domainConfig.SubmitMode},
			RegistryAddress: domainRegistryAddress.String(),
		}
	case *domains.SimpleStorageDomainConfig:
		endorsementSet := make([]string, 1+len(peerNodes))
		endorsementSet[0] = binding.name
		for i, peerNode := range peerNodes {
			endorsementSet[i+1] = peerNode.(*nodeConfiguration).name
		}
		i.conf.DomainManagerConfig.Domains["simpleStorageDomain"] = &pldconf.DomainConfig{
			AllowSigning: true,
			Plugin: pldconf.PluginConfig{
				Type:    string(pldtypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config: map[string]any{
				"submitMode":     domainConfig.SubmitMode,
				"endorsementSet": endorsementSet,
			},
			RegistryAddress: domainRegistryAddress.String(),
		}
	}

	i.conf.NodeName = binding.name
	i.conf.Transports = map[string]*pldconf.TransportConfig{
		"grpc": {
			Plugin: pldconf.PluginConfig{
				Type:    string(pldtypes.LibraryTypeCShared),
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
		nodesConfig[peerNode.(*nodeConfiguration).name] = &static.StaticEntry{
			Properties: map[string]pldtypes.RawJSON{
				"transport.grpc": pldtypes.JSONString(
					grpc.PublishedTransportDetails{
						Endpoint: fmt.Sprintf("dns:///%s:%d", peerNode.(*nodeConfiguration).address, peerNode.(*nodeConfiguration).port),
						Issuers:  peerNode.(*nodeConfiguration).cert,
					},
				),
			},
		}
	}

	i.conf.Registries = map[string]*pldconf.RegistryConfig{
		"registry1": {
			Plugin: pldconf.PluginConfig{
				Type:    string(pldtypes.LibraryTypeCShared),
				Library: "loaded/via/unit/test/loader",
			},
			Config: map[string]any{
				"entries": nodesConfig,
			},
		},
	}

	if i.conf.DB.Type == "postgres" {
		dns, cleanUp := initPostgres(t, context.Background(), binding.name)
		i.conf.DB.Postgres.DSN = dns
		t.Cleanup(cleanUp)
	}

	var pl plugins.UnitTestPluginLoader

	i.cm = componentmgr.NewComponentManager(i.ctx, i.grpcTarget, uuid.New(), i.conf)
	// Start it up
	err = i.cm.Init()
	require.NoError(t, err)

	err = i.cm.StartManagers()
	require.NoError(t, err)

	loaderMap := map[string]plugintk.Plugin{
		"domain1":             domains.SimpleTokenDomain(t, i.ctx),
		"simpleStorageDomain": domains.SimpleStorageDomain(t, i.ctx),
		"grpc":                grpc.NewPlugin(i.ctx),
		"registry1":           static.NewPlugin(i.ctx),
	}
	pc := i.cm.PluginManager()
	pl, err = plugins.NewUnitTestPluginLoader(pc.GRPCTargetURL(), pc.LoaderID().String(), loaderMap)
	i.pluginManager = pl
	require.NoError(t, err)
	go pl.Run()

	err = i.cm.CompleteStart()
	require.NoError(t, err)

	// Coordination tests start and stop nodes during the test, so they
	// manually handle cleaning up the plugin and component managers
	if !manualTestCleanup {
		t.Cleanup(func() {
			pl.Stop()
			i.cm.Stop()
		})
	}

	client, err := rpcclient.NewHTTPClient(log.WithLogField(context.Background(), "client-for", binding.name), &pldconf.HTTPClientConfig{URL: "http://localhost:" + strconv.Itoa(*i.conf.RPCServer.HTTP.Port)})
	require.NoError(t, err)
	i.client = client

	i.resolveEthereumAddress = func(identity string) string {
		idPart, err := pldtypes.PrivateIdentityLocator(identity).Identity(context.Background())
		require.NoError(t, err)
		addr, err := i.cm.KeyManager().ResolveEthAddressNewDatabaseTX(i.ctx, idPart)
		require.NoError(t, err)
		return addr.String()
	}

	return i
}

func initPostgres(t *testing.T, ctx context.Context, nodeName string) (dns string, cleanup func()) {
	dbDSN := func(dbname string) string {
		return fmt.Sprintf("postgres://postgres:my-secret@localhost:5432/%s?sslmode=disable", dbname)
	}
	componentTestdbName := fmt.Sprintf("coordtestbed%s", nodeName)
	log.L(ctx).Infof("Component test Postgres DB: %s", componentTestdbName)

	// First create the database - using the super user
	adminDB, err := sql.Open("postgres", dbDSN("postgres"))
	// Check if the database already exists
	res, err := adminDB.Query(fmt.Sprintf(`SELECT 1 FROM pg_database WHERE datname = '%s';`, componentTestdbName))

	require.NoError(t, err)

	if res != nil && res.Next() {
		log.L(ctx).Infof("Database already exists: %s", componentTestdbName)

		err = adminDB.Close()
		require.NoError(t, err)

		// Don't delete the existing DB after the tests
		return dbDSN(componentTestdbName), func() {}
	}

	// DB doesn't already exist so try to create it
	_, err = adminDB.Exec(fmt.Sprintf(`CREATE DATABASE "%s";`, componentTestdbName))

	if err != nil {
		log.L(ctx).Errorf("Error creating database: %s", err)
		require.NoError(t, err)
	}

	if err == nil {
		err = adminDB.Close()
	}

	// If we created the database to run the tests, delete it at the end
	return dbDSN(componentTestdbName), func() {
		adminDB, err := sql.Open("postgres", dbDSN("postgres"))
		if err == nil {
			_, _ = adminDB.Exec(fmt.Sprintf(`DROP DATABASE "%s" WITH(FORCE);`, componentTestdbName))
			_ = adminDB.Close()
		}
	}
}

func DeployDomainRegistry(t *testing.T, configPath string) *pldtypes.EthAddress {
	// We need an engine so that we can deploy the base ledger contract for the domain
	//Actually, we only need a bare bones engine that is capable of deploying the base ledger contracts
	// could make do with assembling some core components like key manager, eth client factory, block indexer, persistence and any other dependencies they pull in
	// but is easier to just create a throwaway component manager with no domains
	tmpConf, _ := testConfig(t, false, configPath)
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
	domainRegistryAddress := domains.DeploySmartContract(t, cmTmp.Persistence(), cmTmp.TxManager(), cmTmp.KeyManager())

	cmTmp.Stop()
	return domainRegistryAddress

}

func testConfig(t *testing.T, enableWS bool, configPath string) (pldconf.PaladinConfig, pldconf.WSClientConfig) {
	ctx := context.Background()

	var conf *pldconf.PaladinConfig
	err := config.ReadAndParseYAMLFile(ctx, configPath, &conf)
	assert.NoError(t, err)

	// For running in this unit test the dirs are different to the sample config
	// conf.DB.SQLite.DebugQueries = true
	conf.DB.SQLite.MigrationsDir = "../../db/migrations/sqlite"
	// conf.DB.Postgres.DebugQueries = true
	conf.DB.Postgres.MigrationsDir = "../../db/migrations/postgres"

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

	// Postgres config typically passes in a fixed seed so re-runs against the same DB
	// use consistent signing keys. Sqlite in-memory config typically relies on a random
	// seed for each run
	var key string
	parsedKey, err := pldtypes.ParseHexBytes(context.Background(), conf.Wallets[0].Signer.KeyStore.Static.Keys["seed"].Inline)
	if err == nil {
		// Valid hex in the config - use it
		key = parsedKey.HexString()
	} else {
		// No hex or a comment, generate a key to use
		key = pldtypes.RandHex(32)
	}

	conf.Wallets[0].Signer.KeyStore.Static.Keys["seed"] = pldconf.StaticKeyEntryConfig{
		Encoding: "hex",
		Inline:   key,
	}

	conf.Log = pldconf.LogConfig{
		Level:  confutil.P("debug"),
		Output: confutil.P("file"),
		File: pldconf.LogFileConfig{
			Filename: confutil.P("build/testbed.component-test.log"),
		},
	}
	log.InitConfig(&conf.Log)

	initPostgres(t, context.Background(), conf.NodeName)

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

type Party interface {
	GetIdentity() string
	GetName() string
	GetNodeConfig() *nodeConfiguration
	GetClient() rpcclient.Client
	AddPeer(peers ...interface{})
	Start(t *testing.T, domainConfig any, configPath string, manualTestCleanup bool)
	Stop(t *testing.T)
	GetIdentityLocator() string
	DeploySimpleDomainInstanceContract(t *testing.T, endorsementMode string, constructorParameters *domains.ConstructorParameters,
		transactionReceiptCondition func(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool,
		transactionLatencyThreshold func(t *testing.T) time.Duration) *pldtypes.EthAddress
	DeploySimpleStorageDomainInstanceContract(t *testing.T, endorsementMode string, constructorParameters *domains.SimpleStorageConstructorParameters,
		transactionReceiptCondition func(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool,
		transactionLatencyThreshold func(t *testing.T) time.Duration) *pldtypes.EthAddress
	OverrideSequencerConfig(config *pldconf.SequencerConfig)
}

func (p *partyForTesting) GetIdentity() string {
	return p.identity
}

func (p *partyForTesting) GetName() string {
	return p.name
}

func (p *partyForTesting) GetNodeConfig() *nodeConfiguration {
	return p.nodeConfig
}

func (p *partyForTesting) GetClient() rpcclient.Client {
	return p.client
}

func (p *partyForTesting) GetIdentityLocator() string {
	return p.identityLocator
}

func (p *partyForTesting) OverrideSequencerConfig(config *pldconf.SequencerConfig) {
	p.nodeConfig.sequencerConfig = config
}

func (p *partyForTesting) DeploySimpleDomainInstanceContract(t *testing.T, endorsementMode string, constructorParameters *domains.ConstructorParameters, transactionReceiptCondition func(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool, transactionLatencyThreshold func(t *testing.T) time.Duration) *pldtypes.EthAddress {

	var dplyTxID uuid.UUID

	err := p.client.CallRPC(context.Background(), &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleTokenConstructorABI(endorsementMode),
		TransactionBase: pldapi.TransactionBase{
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: "domain1",
			From:   p.identity,
			Data:   pldtypes.JSONString(constructorParameters),
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
	err = p.client.CallRPC(context.Background(), &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	require.NotNil(t, dplyTxFull.Receipt)
	require.True(t, dplyTxFull.Receipt.Success)
	require.NotNil(t, dplyTxFull.Receipt.ContractAddress)
	return dplyTxFull.Receipt.ContractAddress
}

func (p *partyForTesting) DeploySimpleStorageDomainInstanceContract(t *testing.T, endorsementMode string, constructorParameters *domains.SimpleStorageConstructorParameters,
	transactionReceiptCondition func(t *testing.T, ctx context.Context, txID uuid.UUID, rpcClient rpcclient.Client, isDeploy bool) func() bool,
	transactionLatencyThreshold func(t *testing.T) time.Duration) *pldtypes.EthAddress {

	var dplyTxID uuid.UUID

	err := p.client.CallRPC(context.Background(), &dplyTxID, "ptx_sendTransaction", &pldapi.TransactionInput{
		ABI: *domains.SimpleStorageConstructorABI(endorsementMode),
		TransactionBase: pldapi.TransactionBase{
			Type:   pldapi.TransactionTypePrivate.Enum(),
			Domain: "simpleStorageDomain",
			From:   p.identity,
			Data:   pldtypes.JSONString(constructorParameters),
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
	err = p.client.CallRPC(context.Background(), &dplyTxFull, "ptx_getTransactionFull", dplyTxID)
	require.NoError(t, err)
	require.NotNil(t, dplyTxFull.Receipt)
	require.True(t, dplyTxFull.Receipt.Success)
	require.NotNil(t, dplyTxFull.Receipt.ContractAddress)
	return dplyTxFull.Receipt.ContractAddress
}

type partyForTesting struct {
	name                  string
	identity              string // identity used to resolve the verifier on its local node
	identityLocator       string // fully qualified locator for the identity that can be used on other nodes
	instance              ComponentTestInstance
	nodeConfig            *nodeConfiguration
	peers                 []interface{}
	domainRegistryAddress *pldtypes.EthAddress
	client                rpcclient.Client //TODO swap out for pldclient.PaladinClient
}

func NewPartyForTesting(t *testing.T, name string, domainRegistryAddress *pldtypes.EthAddress) *partyForTesting {
	nodeName := name
	party := &partyForTesting{
		name:                  name,
		peers:                 make([]interface{}, 0),
		domainRegistryAddress: domainRegistryAddress,
		identity:              fmt.Sprintf("wallets.org1.%s", name),
		identityLocator:       fmt.Sprintf("wallets.org1.%s@%s", name, nodeName),
	}

	party.nodeConfig = NewNodeConfiguration(t, nodeName)
	return party
}

func (p *partyForTesting) AddPeer(peers ...interface{}) {
	p.peers = append(p.peers, peers...)
}

func (p *partyForTesting) Start(t *testing.T, domainConfig any, configPath string, manualTestCleanup bool) {
	p.instance = NewInstanceForTesting(t, p.domainRegistryAddress, p.nodeConfig, p.peers, domainConfig, false, configPath, manualTestCleanup)
	p.client = p.instance.GetClient()
}

func (p *partyForTesting) Stop(t *testing.T) {
	p.instance.GetComponentManager().Stop()
	p.instance.GetPluginManager().Stop()
}
