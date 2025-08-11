/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	nototypes "github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/ERC20Simple.json
var ERC20SimpleBuildJSON []byte

//go:embed abis/NotoTrackerERC20.json
var NotoTrackerERC20BuildJSON []byte

const node1HttpURL = "http://127.0.0.1:31548"
const node2HttpURL = "http://127.0.0.1:31648"
const node3HttpURL = "http://127.0.0.1:31748"

const node1WebSocketURL = "ws://127.0.0.1:31549"
const node2WebSocketURL = "ws://127.0.0.1:31649"
const node3WebSocketURL = "ws://127.0.0.1:31749"

func withTimeout[T any](do func(ctx context.Context) T) T {
	ctx, cancelCtx := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelCtx()
	return do(ctx)
}

func testLog(message string, inserts ...any) {
	log.L(context.Background()).Warnf(fmt.Sprintf("** TEST OUTPUT **: %s", message), inserts...)
}

func with18Decimals(x int64) *pldtypes.HexUint256 {
	bx := new(big.Int).Mul(
		big.NewInt(x),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), big.NewInt(0)),
	)
	return (*pldtypes.HexUint256)(bx)
}

func with10Decimals(x int64) *pldtypes.HexUint256 {
	bx := new(big.Int).Mul(
		big.NewInt(x),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(10), big.NewInt(0)),
	)
	return (*pldtypes.HexUint256)(bx)
}

// func getJSONPropertyAs(jsonData pldtypes.RawJSON, name string, toValue any) {
// 	var mapProp map[string]pldtypes.RawJSON
// 	err := json.Unmarshal(jsonData, &mapProp)
// 	if err != nil {
// 		panic(fmt.Errorf("Unable to unmarshal %s", jsonData))
// 	}
// 	err = json.Unmarshal(mapProp[name], toValue)
// 	if err != nil {
// 		panic(fmt.Errorf("Unable to map %s to %T: %s", mapProp[name], toValue, err))
// 	}
// }

var pentePrivGroupComps = abi.ParameterArray{
	{Name: "salt", Type: "bytes32"},
	{Name: "members", Type: "string[]"},
}

var penteGroupABI = &abi.Parameter{
	Name: "group", Type: "tuple", Components: pentePrivGroupComps,
}

var penteConstructorABI = &abi.Entry{
	Type: abi.Constructor, Inputs: abi.ParameterArray{
		penteGroupABI,
		{Name: "evmVersion", Type: "string"},
		{Name: "endorsementType", Type: "string"},
		{Name: "externalCallsEnabled", Type: "bool"},
	},
}

type penteConstructorParams struct {
	Group                nototypes.PentePrivateGroup `json:"group"`
	EVMVersion           string                      `json:"evmVersion"`
	EndorsementType      string                      `json:"endorsementType"`
	ExternalCallsEnabled bool                        `json:"externalCallsEnabled"`
}

// This works for both ERC20Simple and NotoTrackerERC20 when invoked via Pente
var erc20PrivateABI = abi.ABI{
	{
		Type: abi.Function,
		Name: "deploy",
		Inputs: abi.ParameterArray{
			penteGroupABI,
			{Name: "bytecode", Type: "bytes"},
			{Name: "inputs", Type: "tuple", Components: abi.ParameterArray{
				{Name: "name", Type: "string"},
				{Name: "symbol", Type: "string"},
			}},
		},
	},
	{
		Type: abi.Function,
		Name: "mint",
		Inputs: abi.ParameterArray{
			penteGroupABI,
			{Name: "to", Type: "address"},
			{Name: "inputs", Type: "tuple", Components: abi.ParameterArray{
				{Name: "to", Type: "address"},
				{Name: "amount", Type: "uint256"},
			}},
		},
	},
	{
		Type: abi.Function,
		Name: "transfer",
		Inputs: abi.ParameterArray{
			penteGroupABI,
			{Name: "to", Type: "address"},
			{Name: "inputs", Type: "tuple", Components: abi.ParameterArray{
				{Name: "to", Type: "address"},
				{Name: "value", Type: "uint256"},
			}},
		},
	},
	{
		Type: abi.Function,
		Name: "balanceOf",
		Inputs: abi.ParameterArray{
			penteGroupABI,
			{Name: "to", Type: "address"},
			{Name: "inputs", Type: "tuple", Components: abi.ParameterArray{
				{Name: "account", Type: "address"},
			}},
		},
		Outputs: abi.ParameterArray{
			{Type: "uint256"},
		},
	},
}

type penteDeployParams struct {
	Group    nototypes.PentePrivateGroup `json:"group"`
	Bytecode pldtypes.HexBytes           `json:"bytecode"`
	Inputs   any                         `json:"inputs"`
}

type penteInvokeParams struct {
	Group  nototypes.PentePrivateGroup `json:"group"`
	To     pldtypes.EthAddress         `json:"to"`
	Inputs any                         `json:"inputs"`
}

type penteReceipt struct {
	Receipt struct {
		ContractAddress *pldtypes.EthAddress `json:"contractAddress"`
	} `json:"receipt"`
}

var _ = Describe("noto/pente - simple", Ordered, func() {
	var listenerName = "e2e_" + uuid.NewString()
	var wsRPC = map[string]pldclient.PaladinWSClient{}

	BeforeAll(func() {
		// Skip("for now")
	})

	AfterAll(func() {
		wsc := wsRPC[paladinPrefix+"1"]
		if wsc != nil {
			_, _ = wsc.PTX().DeleteReceiptListener(context.Background(), listenerName)
		}
		for _, wsc := range wsRPC {
			wsc.Close()
		}
	})

	Context("Noto domain verification", func() {

		ctx := context.Background()
		rpc := map[string]pldclient.PaladinClient{}

		connectNode := func(url, wsURL, name string) {
			Eventually(func() bool {
				return withTimeout(func(ctx context.Context) bool {
					pld, err := pldclient.New().HTTP(ctx, &pldconf.HTTPClientConfig{URL: url})
					if err == nil {
						queriedName, err := pld.Transport().NodeName(ctx)
						Expect(err).To(BeNil())
						Expect(queriedName).To(Equal(name))
						rpc[name] = pld
					}
					return err == nil
				})
			}).Should(BeTrue())

			wsRPCClient, wsErr := pldclient.New().WebSocket(ctx, &pldconf.WSClientConfig{HTTPClientConfig: pldconf.HTTPClientConfig{URL: wsURL}})
			if wsErr == nil {
				queriedName, err := wsRPCClient.Transport().NodeName(ctx)
				Expect(err).To(BeNil())
				Expect(queriedName).To(Equal(name))
				wsRPC[name] = wsRPCClient
			}
		}

		It("waits to connect to all three nodes", func() {
			connectNode(node1HttpURL, node1WebSocketURL, paladinPrefix+"1")
			connectNode(node2HttpURL, node2WebSocketURL, paladinPrefix+"2")
			connectNode(node3HttpURL, node3WebSocketURL, paladinPrefix+"3")
		})

		It("checks nodes can talk to each other", func() {
			for src := range rpc {
				for dest := range rpc {
					Eventually(func() bool {
						return withTimeout(func(ctx context.Context) bool {
							verifier, err := rpc[src].PTX().ResolveVerifier(ctx, fmt.Sprintf("test@%s", dest),
								algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
							if err == nil {
								addr, err := pldtypes.ParseEthAddress(verifier)
								Expect(err).To(BeNil())
								Expect(addr).ToNot(BeNil())
							}
							return err == nil
						})
					}).Should(BeTrue())
				}
			}
		})

		var notary = fmt.Sprintf("notary.on@%s1", paladinPrefix)
		var notoContract *pldtypes.EthAddress
		var deploySequenceNode1 uint64

		It("deploys a noto", func() {
			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, abi.ABI{
				{Type: abi.Constructor, Inputs: abi.ParameterArray{
					{Name: "notary", Type: "string"},
					{Name: "notaryMode", Type: "string"},
				}},
			}).
				Private().
				Domain("noto").
				Constructor().
				From(notary).
				Inputs(&nototypes.ConstructorParams{
					Notary:     notary,
					NotaryMode: nototypes.NotaryModeBasic,
				}).
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			notoContract = deploy.Receipt().ContractAddress
			deploySequenceNode1 = deploy.Receipt().Sequence
			testLog("Noto (plain) contract %s deployed by TX %s", notoContract, deploy.ID())
		})

		It("creates a receipt listener", func() {
			_, err := rpc[paladinPrefix+"1" /* must match deploySequenceNode1 */].PTX().CreateReceiptListener(ctx, &pldapi.TransactionReceiptListener{
				Name: listenerName,
				Filters: pldapi.TransactionReceiptFilters{
					// Just listen to receipts from this point on
					SequenceAbove: confutil.P(deploySequenceNode1),
				},
				Options: pldapi.TransactionReceiptListenerOptions{
					// We just want to check we get all receipts
					IncompleteStateReceiptBehavior: pldapi.IncompleteStateReceiptBehaviorProcess.Enum(),
				},
			})
			Expect(err).To(BeNil())
		})

		var receiptsSub rpcclient.Subscription
		It("starts a receipt listener", func() {
			sub, err := wsRPC[paladinPrefix+"1"].PTX().SubscribeReceipts(ctx, listenerName)
			Expect(err).To(BeNil())
			receiptsSub = sub
		})

		var notoCoinSchemaID *pldtypes.Bytes32
		It("gets the coin schema", func() {
			var schemas []*pldapi.Schema
			err := rpc[paladinPrefix+"1"].CallRPC(ctx, &schemas, "pstate_listSchemas", "noto")
			Expect(err).To(BeNil())
			for _, s := range schemas {
				if s.Signature == "type=NotoCoin(bytes32 salt,string owner,uint256 amount),labels=[owner,amount]" {
					notoCoinSchemaID = &s.ID
				}
			}
			Expect(notoCoinSchemaID).ToNot(BeNil())
		})

		logWallet := func(identity, node string) {
			var addr *pldtypes.EthAddress
			err := rpc[node].CallRPC(ctx, &addr, "keymgr_resolveEthAddress", identity)
			Expect(err).To(BeNil())
			var coins []*nototypes.NotoCoinState
			err = rpc[node].CallRPC(ctx, &coins, "pstate_queryContractStates", "noto", notoContract, notoCoinSchemaID,
				query.NewQueryBuilder().Equal("owner", addr).Limit(100).Query(),
				"available")
			Expect(err).To(BeNil())
			balance := big.NewInt(0)
			summary := make([]string, len(coins))
			for ic, c := range coins {
				summary[ic] = fmt.Sprintf("%s...[%s]", c.ID.String()[0:8], c.Data.Amount.Int().Text(10))
				balance = new(big.Int).Add(balance, c.Data.Amount.Int())
			}
			testLog("%s@%s balance=%s coins:%v", identity, node, balance, summary)
		}

		It("mints some notos to bob on node1", func() {
			for _, amount := range []*pldtypes.HexUint256{
				with18Decimals(15),
				with18Decimals(25), // 40
				with18Decimals(30), // 70
				with18Decimals(42), // 112
			} {
				txn := rpc[paladinPrefix+"1"].ForABI(ctx, nototypes.NotoABI).
					Private().
					Domain("noto").
					Function("mint").
					To(notoContract).
					From(notary).
					Inputs(&nototypes.MintParams{
						To:     fmt.Sprintf("bob@%s1", paladinPrefix),
						Amount: amount,
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Noto mint transaction %s", txn.ID())
				Expect(txn.Error()).To(BeNil())
				logWallet("bob", paladinPrefix+"1")
			}
		})

		It("sends some notos to sally on node2", func() {
			for _, amount := range []*pldtypes.HexUint256{
				with18Decimals(33), // 79
				with18Decimals(66), // 13
			} {
				txn := rpc[paladinPrefix+"1"].ForABI(ctx, nototypes.NotoABI).
					Private().
					Domain("noto").
					Function("transfer").
					To(notoContract).
					From(fmt.Sprintf("bob@%s1", paladinPrefix)).
					Inputs(&nototypes.TransferParams{
						To:     fmt.Sprintf("sally@%s2", paladinPrefix),
						Amount: amount,
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Noto transfer transaction %s", txn.ID())
				Expect(txn.Error()).To(BeNil())
				logWallet("bob", paladinPrefix+"1")
				logWallet("sally", paladinPrefix+"2")
			}
		})

		It("sally on node2 sends some notos to fred on node3", func() {
			txn := rpc[paladinPrefix+"2"].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				Function("transfer").
				To(notoContract).
				From(fmt.Sprintf("sally@%s2", paladinPrefix)).
				Inputs(&nototypes.TransferParams{
					To:     fmt.Sprintf("fred@%s3", paladinPrefix),
					Amount: with18Decimals(6),
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Noto transfer transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("sally", paladinPrefix+"2")
			logWallet("fred", paladinPrefix+"3")
			testLog("done testing noto in isolation")
		})

		penteGroupNodes1and2 := nototypes.PentePrivateGroup{
			Salt:    pldtypes.RandBytes32(),                                                                   // unique salt must be shared privately to retain anonymity
			Members: []string{fmt.Sprintf("bob@%s1", paladinPrefix), fmt.Sprintf("sally@%s2", paladinPrefix)}, // these will be salted to establish the endorsement key identifiers
		}

		var penteContract *pldtypes.EthAddress
		It("deploys a pente privacy group to node1 and node2, excluding node3", func() {

			const ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES = "group_scoped_identities"

			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, abi.ABI{penteConstructorABI}).
				Private().
				Domain("pente").
				Constructor().
				Inputs(&penteConstructorParams{
					Group:                penteGroupNodes1and2,
					EVMVersion:           "shanghai",
					EndorsementType:      ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
					ExternalCallsEnabled: true,
				}).
				From("random." + uuid.NewString()). // anyone can submit this by design
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			penteContract = deploy.Receipt().ContractAddress
			testLog("Pente privacy group %s (salt=%s) deployed by TX %s", penteContract, penteGroupNodes1and2.Salt, deploy.ID())
		})

		erc20Simple := solutils.MustLoadBuild(ERC20SimpleBuildJSON)
		var erc20DeployID uuid.UUID
		It("deploys a vanilla ERC-20 into the the privacy group with a minter/owner", func() {

			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, erc20PrivateABI).
				Private().
				Domain("pente").
				To(penteContract).
				Function("deploy").
				Inputs(&penteDeployParams{
					Group:    penteGroupNodes1and2,
					Bytecode: erc20Simple.Bytecode,
					Inputs: map[string]any{
						"name":   "Stars",
						"symbol": "STAR",
					},
				}).
				From(fmt.Sprintf("seren@%s1", paladinPrefix)).
				Send().
				Wait(5 * time.Second)
			testLog("Deployed SimpleERC20 contract into privacy group in transaction %s", deploy.ID())
			Expect(deploy.Error()).To(BeNil())
			erc20DeployID = deploy.ID()
		})

		var erc20StarsAddr *pldtypes.EthAddress
		It("requests the receipt from pente to get the contract address", func() {

			domainReceiptJSON, err := rpc[paladinPrefix+"1"].PTX().GetDomainReceipt(ctx, "pente", erc20DeployID)
			Expect(err).To(BeNil())
			var pr penteReceipt
			err = json.Unmarshal(domainReceiptJSON, &pr)
			Expect(err).To(BeNil())
			erc20StarsAddr = pr.Receipt.ContractAddress
			testLog("SimpleERC20 contractAddress (within privacy group): %s", erc20StarsAddr)

		})

		getERC20Balance := func(identity, node string) *pldtypes.HexUint256 {
			addr := getEthAddress(ctx, rpc[node], identity, node)
			type ercBalanceOf struct {
				Param0 *pldtypes.HexUint256 `json:"0"`
			}
			var result ercBalanceOf
			err := rpc[node].ForABI(ctx, erc20PrivateABI).
				Private().
				Domain("pente").
				To(penteContract).
				Function("balanceOf").
				Inputs(&penteInvokeParams{
					Group: penteGroupNodes1and2,
					To:    *erc20StarsAddr,
					Inputs: map[string]any{
						"account": addr.String(),
					},
				}).
				Outputs(&result).
				From(fmt.Sprintf("%s@%s", identity, node)).
				Call()
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			return result.Param0
		}

		It("mints some ERC-20 inside the the privacy group", func() {

			invoke := rpc[paladinPrefix+"1"].ForABI(ctx, erc20PrivateABI).
				Private().
				Domain("pente").
				To(penteContract).
				Function("mint").
				Inputs(&penteInvokeParams{
					Group: penteGroupNodes1and2,
					To:    *erc20StarsAddr,
					Inputs: map[string]any{
						"to":     getEthAddress(ctx, rpc[paladinPrefix+"1"], "seren", paladinPrefix+"1"),
						"amount": with18Decimals(1977),
					},
				}).
				From(fmt.Sprintf("seren@%s1", paladinPrefix)).
				Send().
				Wait(5 * time.Second)
			testLog("SimpleERC20 mint transaction %s", invoke.ID())
			Expect(invoke.Error()).To(BeNil())

		})

		It("check ERC-20 balance of Seren", func() {

			serenBalance := getERC20Balance("seren", paladinPrefix+"1")
			testLog("SimpleERC20 balance after mint to seren@%s1: %s", paladinPrefix, serenBalance.Int())
			Expect(serenBalance.String()).To(Equal(with18Decimals(1977).String()))

		})

		var erc20TransferID uuid.UUID
		It("transfers some ERC-20 inside the the privacy group from seren@node1 to sally@node2", func() {

			invoke := rpc[paladinPrefix+"1"].ForABI(ctx, erc20PrivateABI).
				Private().
				Domain("pente").
				To(penteContract).
				Function("transfer").
				Inputs(&penteInvokeParams{
					Group: penteGroupNodes1and2,
					To:    *erc20StarsAddr,
					Inputs: map[string]any{
						"to":    getEthAddress(ctx, rpc[paladinPrefix+"2"], "sally", paladinPrefix+"2"),
						"value": with18Decimals(42),
					},
				}).
				From(fmt.Sprintf("seren@%s1", paladinPrefix)).
				Send().
				Wait(5 * time.Second)
			testLog("SimpleERC20 mint transaction %s", invoke.ID())
			Expect(invoke.Error()).To(BeNil())
			erc20TransferID = invoke.ID()

		})

		decodePrivateEVMEvent := func(eventDef *abi.Entry, log *pldapi.PrivateEVMLog) string {
			ethTopics := make([]ethtypes.HexBytes0xPrefix, len(log.Topics))
			for i, t := range log.Topics {
				ethTopics[i] = t[:]
			}
			cv, err := eventDef.DecodeEventDataCtx(ctx, ethTopics, ethtypes.HexBytes0xPrefix(log.Data))
			Expect(err).To(BeNil())
			b, err := pldtypes.DefaultJSONFormatOptions.GetABISerializerIgnoreErrors(ctx).SerializeJSONCtx(ctx, cv)
			Expect(err).To(BeNil())
			return string(b)
		}

		It("waits for the receipt logs on node2", func() {
			var penteReceiptJSON pldtypes.RawJSON
			Eventually(func() error {
				var err error
				penteReceiptJSON, err = rpc[paladinPrefix+"2"].PTX().GetDomainReceipt(ctx, "pente", erc20TransferID)
				return err
			}, "5s").Should(BeNil())
			var penteReceipt *pldapi.PenteDomainReceipt
			err := json.Unmarshal(penteReceiptJSON, &penteReceipt)
			Expect(err).To(BeNil())

			// Decode the transfer
			erc20TransferABI := erc20Simple.ABI.Events()["Transfer"]
			Expect(penteReceipt.Receipt.Logs).To(HaveLen(1))
			transferEventJSON := decodePrivateEVMEvent(erc20TransferABI, penteReceipt.Receipt.Logs[0])
			Expect(transferEventJSON).To(MatchJSON(fmt.Sprintf(`{
				"from": "%s",
				"to": "%s",
				"value": "42000000000000000000"
			}`, getEthAddress(ctx, rpc[paladinPrefix+"1"], "seren", paladinPrefix+"1"), getEthAddress(ctx, rpc[paladinPrefix+"2"], "sally", paladinPrefix+"2"))))
		})

		It("check ERC-20 balance of Seren and Sally", func() {

			serenBalance := getERC20Balance("seren", paladinPrefix+"1")
			testLog("SimpleERC20 balance after mint to seren@%s1: %s", paladinPrefix, serenBalance.Int())
			Expect(serenBalance.String()).To(Equal(with18Decimals(1935).String()))

			sallyBalance := getERC20Balance("sally", paladinPrefix+"2")
			testLog("SimpleERC20 balance after mint to seren@%s1: %s", paladinPrefix, sallyBalance.Int())
			Expect(sallyBalance.String()).To(Equal(with18Decimals(42).String()))

		})

		var notoTrackerDeployTX uuid.UUID
		It("deploys a noto tracker smart contract into the privacy group", func() {

			notoTracker := solutils.MustLoadBuild(NotoTrackerERC20BuildJSON)

			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, erc20PrivateABI).
				Private().
				Domain("pente").
				To(penteContract).
				Function("deploy").
				Inputs(&penteDeployParams{
					Group:    penteGroupNodes1and2,
					Bytecode: notoTracker.Bytecode,
					Inputs: map[string]any{
						"name":   "NOTO",
						"symbol": "NOTO",
					},
				}).
				From(notary).
				Send().
				Wait(5 * time.Second)
			testLog("Deployed NotoTrackerERC20 contract into privacy group in transaction %s", deploy.ID())
			Expect(deploy.Error()).To(BeNil())
			notoTrackerDeployTX = deploy.ID()
		})

		var notoTrackerAddr *pldtypes.EthAddress
		It("requests the receipt from pente to get the contract address", func() {

			domainReceiptJSON, err := rpc[paladinPrefix+"1"].PTX().GetDomainReceipt(ctx, "pente", notoTrackerDeployTX)
			Expect(err).To(BeNil())
			var pr penteReceipt
			err = json.Unmarshal(domainReceiptJSON, &pr)
			Expect(err).To(BeNil())
			notoTrackerAddr = pr.Receipt.ContractAddress
			testLog("NotoTrackerERC20 contractAddress (within privacy group): %s", erc20StarsAddr)

		})

		var notoPenteContractAddr *pldtypes.EthAddress
		It("deploys a new noto using Pente smart contract as the notary", func() {
			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, abi.ABI{
				{Type: abi.Constructor, Inputs: abi.ParameterArray{
					{Name: "notary", Type: "string"},
					{Name: "notaryMode", Type: "string"},
					{Name: "options", Type: "tuple", Components: abi.ParameterArray{
						{Name: "hooks", Type: "tuple", Components: abi.ParameterArray{
							{Name: "publicAddress", Type: "string"},
							{Name: "privateAddress", Type: "string"},
							{Name: "privateGroup", Type: "tuple", Components: pentePrivGroupComps},
						}},
					}},
				}},
			}).
				Private().
				Domain("noto").
				Constructor().
				From(notary).
				Inputs(&nototypes.ConstructorParams{
					Notary:     notary,
					NotaryMode: nototypes.NotaryModeHooks,
					Options: nototypes.NotoOptions{
						Hooks: &nototypes.NotoHooksOptions{
							PublicAddress:  penteContract,
							PrivateAddress: notoTrackerAddr,
							PrivateGroup:   &penteGroupNodes1and2,
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			notoPenteContractAddr = deploy.Receipt().ContractAddress
			testLog("Combined Noto<->Pente contract %s deployed by TX %s", notoPenteContractAddr, deploy.ID())
		})

		var mintTxID uuid.UUID
		It("mints some noto-pentes to bob on node1", func() {
			txn := rpc[paladinPrefix+"1"].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				Function("mint").
				To(notoPenteContractAddr).
				From(notary).
				Inputs(&nototypes.MintParams{
					To:     fmt.Sprintf("bob@%s1", paladinPrefix),
					Amount: with18Decimals(99),
				}).
				Send().
				Wait(5 * time.Second)
			Expect(txn.Error()).To(BeNil())
			mintTxID = txn.ID()
			testLog("Noto<->Pente mint transaction %s", txn.ID())
			logWallet("bob", paladinPrefix+"1")
		})

		It("prepares a transfer for some noto-pentes from bob to sally, without submitting to the chain", func() {
			prepared := rpc[paladinPrefix+"1"].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				Function("transfer").
				To(notoPenteContractAddr).
				From(fmt.Sprintf("bob@%s1", paladinPrefix)).
				Inputs(&nototypes.MintParams{
					To:     fmt.Sprintf("sally@%s2", paladinPrefix),
					Amount: with18Decimals(13),
				}).
				Prepare().
				Wait(5 * time.Second)
			Expect(prepared.Error()).To(BeNil())
			testLog("Noto<->Pente prepared transaction original TX id=%s prepared TX idempotencyKey=%v", prepared.ID(), prepared.PreparedTransaction().Transaction.IdempotencyKey)
		})

		It("streams down receipts until it finds the mint", func() {
			mintMatched := false
			for !mintMatched {
				var receiptBatch pldapi.TransactionReceiptBatch
				var notification rpcclient.RPCSubscriptionNotification
				select {
				case notification = <-receiptsSub.Notifications():
					err := json.Unmarshal(notification.GetResult(), &receiptBatch)
					Expect(err).To(BeNil())
				case <-time.After(5 * time.Second):
					Fail("Timed out waiting for ereceipt")
				}
				for _, receipt := range receiptBatch.Receipts {
					testLog("streamed receipt %s", receipt.ID)
					if receipt.ID == mintTxID {
						testLog("matched mint receipt %s", receipt.ID)
						mintMatched = true
					}
				}
				err := notification.Ack(ctx)
				Expect(err).To(BeNil())
			}
		})
	})
})

func getEthAddress(ctx context.Context, rpc pldclient.PaladinClient, identity, node string) pldtypes.EthAddress {
	addr, err := rpc.PTX().ResolveVerifier(ctx, fmt.Sprintf("%s@%s", identity, node), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
	Expect(err).To(BeNil())
	return *pldtypes.MustEthAddress(addr)
}
