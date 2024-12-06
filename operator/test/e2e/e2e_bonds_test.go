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
	"time"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaleido-io/paladin/config/pkg/confutil"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	nototypes "github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/solutils"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/BondTrackerPublic.json
var BondTrackerPublicBuildJSON []byte

//go:embed abis/BondTracker.json
var BondTrackerPrivateBuildJSON []byte

//go:embed abis/InvestorList.json
var InvestorListPrivateBuildJSON []byte

//go:embed abis/BondSubscription.json
var BondSubscriptionPrivateBuildJSON []byte

//go:embed abis/AtomFactory.json
var AtomFactoryBuildJSON []byte

func penteDeployABI(constructor *abi.Entry) abi.ABI {
	return abi.ABI{
		{
			Type: abi.Function,
			Name: "deploy",
			Inputs: abi.ParameterArray{
				penteGroupABI,
				{Name: "bytecode", Type: "bytes"},
				{Name: "inputs", Type: "tuple", Components: constructor.Inputs},
			},
		},
	}
}

func penteMethodABI(method *abi.Entry) abi.ABI {
	return abi.ABI{
		{
			Type: abi.Function,
			Name: method.Name,
			Inputs: abi.ParameterArray{
				penteGroupABI,
				{Name: "to", Type: "address"},
				{Name: "inputs", Type: "tuple", Components: method.Inputs},
			},
			Outputs: method.Outputs,
		},
	}
}

var _ = Describe("controller", Ordered, func() {
	defer GinkgoRecover()

	BeforeAll(func() {
		// Skip("for now")
	})

	AfterAll(func() {
	})

	Context("Bonds", func() {

		ctx := context.Background()
		rpc := map[string]pldclient.PaladinClient{}

		connectNode := func(url, name string) {
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
		}

		It("waits to connect to all three nodes", func() {
			connectNode(node1HttpURL, "node1")
			connectNode(node2HttpURL, "node2")
			connectNode(node3HttpURL, "node3")
		})

		It("checks nodes can talk to each other", func() {
			for src := range rpc {
				for dest := range rpc {
					Eventually(func() bool {
						return withTimeout(func(ctx context.Context) bool {
							verifier, err := rpc[src].PTX().ResolveVerifier(ctx, fmt.Sprintf("test@%s", dest),
								algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
							if err == nil {
								addr, err := tktypes.ParseEthAddress(verifier)
								Expect(err).To(BeNil())
								Expect(addr).ToNot(BeNil())
							}
							return err == nil
						})
					}).Should(BeTrue())
				}
			}
		})

		pentePrivGroupComps := abi.ParameterArray{
			{Name: "salt", Type: "bytes32"},
			{Name: "members", Type: "string[]"},
		}
		penteGroupABI := &abi.Parameter{
			Name: "group", Type: "tuple", Components: pentePrivGroupComps,
		}

		penteConstructorABI := &abi.Entry{
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

		type penteReceipt struct {
			Receipt struct {
				ContractAddress *tktypes.EthAddress `json:"contractAddress"`
			} `json:"receipt"`
		}

		type participant struct {
			id       string
			identity string
			node     string
			addr     tktypes.EthAddress
		}

		bondTrackerPublicBuild := solutils.MustLoadBuild(BondTrackerPublicBuildJSON)
		bondTrackerPrivateBuild := solutils.MustLoadBuild(BondTrackerPrivateBuildJSON)
		investorListPrivateBuild := solutils.MustLoadBuild(InvestorListPrivateBuildJSON)
		bondSubscriptionPrivateBuild := solutils.MustLoadBuild(BondSubscriptionPrivateBuildJSON)
		atomFactoryBuild := solutils.MustLoadBuild(AtomFactoryBuildJSON)

		var cashIssuer, bondIssuer, bondCustodian, alice *participant

		type privacyGroup struct {
			deployNode string
			nototypes.PentePrivateGroup
			contractAddress *tktypes.EthAddress
		}

		var privacyGroups map[string]*privacyGroup

		It("resolves participants", func() {
			resolveParticipant := func(identity string, node string) *participant {
				addr, err := rpc[node].PTX().ResolveVerifier(ctx, fmt.Sprintf("%s@%s", identity, node), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
				if err != nil {
					panic("err")
				}
				return &participant{id: fmt.Sprintf("%s@%s", identity, node), identity: identity, node: node, addr: *tktypes.MustEthAddress(addr)}
			}

			cashIssuer = resolveParticipant("cashIssuer", "node1")
			bondIssuer = resolveParticipant("bondIssuer", "node1")
			bondCustodian = resolveParticipant("bondCustodian", "node2")
			alice = resolveParticipant("alice", "node3")

			privacyGroups = map[string]*privacyGroup{
				"issuerCustodian": {
					deployNode: bondIssuer.node,
					PentePrivateGroup: nototypes.PentePrivateGroup{
						Salt:    tktypes.Bytes32(tktypes.RandBytes(32)),
						Members: []string{bondIssuer.id, bondCustodian.id},
					},
				},
				"aliceCustodian": {
					deployNode: alice.node,
					PentePrivateGroup: nototypes.PentePrivateGroup{
						Salt:    tktypes.Bytes32(tktypes.RandBytes(32)),
						Members: []string{alice.id, bondCustodian.id},
					},
				},
			}
		})

		It("creates privacy groups", func() {
			for name, group := range privacyGroups {
				const ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES = "group_scoped_identities"
				tx := rpc[group.deployNode].ForABI(ctx, abi.ABI{penteConstructorABI}).
					Private().
					Domain("pente").
					Constructor().
					From("random." + uuid.NewString()). // anyone can submit this by design
					Inputs(&penteConstructorParams{
						Group:                group.PentePrivateGroup,
						EVMVersion:           "shanghai",
						EndorsementType:      ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES,
						ExternalCallsEnabled: true,
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Pente privacy group %s (salt=%s) deployment TX %s", name, group.Salt, tx.ID())
				Expect(tx.Error()).To(BeNil())
				Expect(tx.Receipt().ContractAddress).ToNot(BeNil())
				group.contractAddress = tx.Receipt().ContractAddress
				testLog("Pente privacy group %s address: %s", name, group.contractAddress)
			}
		})

		var cashTokenContract *tktypes.EthAddress
		It("creates cash issuer token", func() {
			tx := rpc[cashIssuer.node].ForABI(ctx, abi.ABI{
				{Type: abi.Constructor, Inputs: abi.ParameterArray{
					{Name: "notary", Type: "string"},
				}},
			}).
				Private().
				Domain("noto").
				Constructor().
				From(cashIssuer.identity).
				Inputs(&nototypes.ConstructorParams{
					Notary: cashIssuer.id,
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Cash issuer contract deployment transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			Expect(tx.Receipt().ContractAddress).ToNot(BeNil())
			cashTokenContract = tx.Receipt().ContractAddress
			testLog("Cash issuer contract (noto): %s", cashTokenContract)
		})

		var publicBondTrackerContract *tktypes.EthAddress
		It("deploys the public bond tracker on the base ledger (controlled by the privacy group)", func() {
			tx := rpc[bondIssuer.node].ForABI(ctx, bondTrackerPublicBuild.ABI).
				Public().
				Constructor().
				Bytecode(bondTrackerPublicBuild.Bytecode).
				From(bondIssuer.identity).
				Inputs(map[string]any{
					"owner":          privacyGroups["issuerCustodian"].contractAddress,
					"issueDate_":     "0",
					"maturityDate_":  "1",
					"currencyToken_": cashTokenContract,
					"faceValue_":     "1",
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Bond tracker contract deployment transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			Expect(tx.Receipt().ContractAddress).ToNot(BeNil())
			publicBondTrackerContract = tx.Receipt().ContractAddress
			testLog("Bond tracker (public base ledger): %s", publicBondTrackerContract)
		})

		var privateBondTrackerDeployID uuid.UUID
		It("deploy private bond tracker to the issuer/custodian privacy group", func() {
			method := bondTrackerPrivateBuild.ABI.Constructor()
			tx := rpc[bondIssuer.node].ForABI(ctx, penteDeployABI(method)).
				Private().
				Domain("pente").
				To(privacyGroups["issuerCustodian"].contractAddress).
				From(bondIssuer.identity).
				Function("deploy").
				Inputs(map[string]any{
					"group":    privacyGroups["issuerCustodian"].PentePrivateGroup,
					"bytecode": bondTrackerPrivateBuild.Bytecode.String(),
					"inputs": map[string]any{
						"name":          "BOND",
						"symbol":        "BOND",
						"custodian":     bondCustodian.addr,
						"publicTracker": publicBondTrackerContract,
					},
				}).
				Send().
				Wait(5 * time.Second)
			privateBondTrackerDeployID = tx.ID()
			testLog("Bond tracker Private EVM contract deployment transaction: %s", privateBondTrackerDeployID)
			Expect(tx.Error()).To(BeNil())
		})

		var privateBondTrackerContract *tktypes.EthAddress
		It("requests the receipt from pente to get the contract address", func() {
			domainReceiptJSON, err := rpc[bondIssuer.node].PTX().GetDomainReceipt(ctx, "pente", privateBondTrackerDeployID)
			Expect(err).To(BeNil())
			var pr penteReceipt
			err = json.Unmarshal(domainReceiptJSON, &pr)
			Expect(err).To(BeNil())
			privateBondTrackerContract = pr.Receipt.ContractAddress
			testLog("Bond tracker Private EVM contract: %s", privateBondTrackerContract)
		})

		var bondTokenContract *tktypes.EthAddress
		It("creates bond token contract token", func() {
			tx := rpc[bondCustodian.node].ForABI(ctx, abi.ABI{
				{Type: abi.Constructor, Inputs: abi.ParameterArray{
					{Name: "notary", Type: "string"},
					{Name: "hooks", Type: "tuple", Components: abi.ParameterArray{
						{Name: "privateGroup", Type: "tuple", Components: pentePrivGroupComps},
						{Name: "publicAddress", Type: "address"},
						{Name: "privateAddress", Type: "address"},
					}},
					{Name: "restrictMinting", Type: "bool"},
				}},
			}).
				Private().
				Domain("noto").
				Constructor().
				From(bondCustodian.identity).
				Inputs(&nototypes.ConstructorParams{
					Notary: bondCustodian.id,
					Hooks: &nototypes.HookParams{
						PublicAddress:  privacyGroups["issuerCustodian"].contractAddress,
						PrivateGroup:   &privacyGroups["issuerCustodian"].PentePrivateGroup,
						PrivateAddress: privateBondTrackerContract,
					},
					RestrictMinting: confutil.P(false),
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Bond token contract deployment transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			Expect(tx.Receipt().ContractAddress).ToNot(BeNil())
			bondTokenContract = tx.Receipt().ContractAddress
			testLog("Bond token contract (noto): %s", bondTokenContract)
		})

		It("issues cash to investors", func() {
			tx := rpc[cashIssuer.node].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				To(cashTokenContract).
				From(cashIssuer.id).
				Function("mint").
				Inputs(map[string]any{
					"to":     alice.id,
					"amount": with18Decimals(100000),
					"data":   "0x",
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Cash issuance to alice transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
		})

		It("issues bond to custodian", func() {
			tx := rpc[bondIssuer.node].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				To(bondTokenContract).
				From(bondIssuer.id).
				Function("mint").
				Inputs(map[string]any{
					"to":     bondCustodian.id,
					"amount": 1000,
					"data":   "0x",
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Bond issuance to custodian transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
		})

		It("begins distribution", func() {
			method := bondTrackerPrivateBuild.ABI.Functions()["beginDistribution"]
			tx := rpc[bondIssuer.node].ForABI(ctx, penteMethodABI(method)).
				Private().
				Domain("pente").
				To(privacyGroups["issuerCustodian"].contractAddress).
				From(bondIssuer.identity).
				Function(method.Name).
				Inputs(map[string]any{
					"group": privacyGroups["issuerCustodian"].PentePrivateGroup,
					"to":    privateBondTrackerContract,
					"inputs": map[string]any{
						"discountPrice":       1,
						"minimumDenomination": 1,
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Bond tracker Private EVM contract beginDistribution() invoke txn: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
		})

		var investorListAddress *tktypes.EthAddress
		It("gets the investor registry", func() {
			var out tktypes.RawJSON
			method := bondTrackerPrivateBuild.ABI.Functions()["investorList"]
			err := rpc[bondIssuer.node].ForABI(ctx, penteMethodABI(method)).
				Private().
				Domain("pente").
				To(privacyGroups["issuerCustodian"].contractAddress).
				From(bondIssuer.identity).
				Function(method.Name).
				Inputs(map[string]any{
					"group":  privacyGroups["issuerCustodian"].PentePrivateGroup,
					"to":     privateBondTrackerContract,
					"inputs": map[string]any{},
				}).
				Outputs(&out).
				Call()
			Expect(err).To(BeNil())
			testLog("Bond tracker Private EVM contract investorList() call: %s", out)
			getJSONPropertyAs(out, "0", &investorListAddress)
			Expect(investorListAddress).ToNot(BeNil())
		})

		It("adds alice as an investor", func() {
			method := investorListPrivateBuild.ABI.Functions()["addInvestor"]
			tx := rpc[bondCustodian.node].ForABI(ctx, penteMethodABI(method)).
				Private().
				Domain("pente").
				To(privacyGroups["issuerCustodian"].contractAddress).
				From(bondCustodian.id).
				Function(method.Name).
				Inputs(map[string]any{
					"group": privacyGroups["issuerCustodian"].PentePrivateGroup,
					"to":    investorListAddress,
					"inputs": map[string]any{
						"addr": alice.addr,
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Bond tracker Private EVM contract addInvestor(alice) invoke txn: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
		})

		var atomFactoryContract *tktypes.EthAddress
		It("deploys the atom factory on the base ledger", func() {
			tx := rpc[bondIssuer.node].ForABI(ctx, atomFactoryBuild.ABI).
				Public().
				Constructor().
				Bytecode(atomFactoryBuild.Bytecode).
				From(bondIssuer.identity).
				Inputs(map[string]any{}).
				Send().
				Wait(5 * time.Second)
			testLog("Atom factory contract deployment transaction: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			Expect(tx.Receipt().ContractAddress).ToNot(BeNil())
			atomFactoryContract = tx.Receipt().ContractAddress
			testLog("Atom factory (public base ledger): %s", atomFactoryContract)
		})

		var privateBondSubscriptionDeployID uuid.UUID
		It("deploy private bond subscription to the alice/custodian privacy group", func() {
			method := bondSubscriptionPrivateBuild.ABI.Constructor()
			tx := rpc[alice.node].ForABI(ctx, penteDeployABI(method)).
				Private().
				Domain("pente").
				To(privacyGroups["aliceCustodian"].contractAddress).
				From(alice.identity).
				Function("deploy").
				Inputs(map[string]any{
					"group":    privacyGroups["aliceCustodian"].PentePrivateGroup,
					"bytecode": bondSubscriptionPrivateBuild.Bytecode.String(),
					"inputs": map[string]any{
						"bondAddress_": bondTokenContract,
						"units_":       1000,
						"custodian_":   bondCustodian.addr,
						"atomFactory_": atomFactoryContract,
					},
				}).
				Send().
				Wait(5 * time.Second)
			privateBondSubscriptionDeployID = tx.ID()
			testLog("Bond subscription Private EVM contract deployment transaction: %s", privateBondSubscriptionDeployID)
			Expect(tx.Error()).To(BeNil())
		})

		var privateBondSubscriptionContract *tktypes.EthAddress
		It("requests the receipt from pente to get the contract address", func() {
			domainReceiptJSON, err := rpc[alice.node].PTX().GetDomainReceipt(ctx, "pente", privateBondSubscriptionDeployID)
			Expect(err).To(BeNil())
			var pr penteReceipt
			err = json.Unmarshal(domainReceiptJSON, &pr)
			Expect(err).To(BeNil())
			privateBondSubscriptionContract = pr.Receipt.ContractAddress
			testLog("Bond subscription Private EVM contract: %s", privateBondSubscriptionContract)
		})

		var preparedBondTransfer *pldapi.PreparedTransaction
		It("prepare transfer of bonds from custodian to alice", func() {
			tx := rpc[bondCustodian.node].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				To(bondTokenContract).
				From(bondCustodian.id).
				Function("transfer").
				Inputs(map[string]any{
					"to":     alice.id,
					"amount": 1000,
					"data":   "0x",
				}).
				Prepare().
				Wait(5 * time.Second)
			testLog("Bond transfer to alice prepared TX ID: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			preparedBondTransfer = tx.PreparedTransaction()
			Expect(preparedBondTransfer).ToNot(BeNil())
			Expect(preparedBondTransfer.Domain).ToNot(Equal("pente"))
		})

		var preparedPaymentTransfer *pldapi.PreparedTransaction
		It("prepare transfer of cash from alice to custodian", func() {
			tx := rpc[alice.node].ForABI(ctx, nototypes.NotoABI).
				Private().
				Domain("noto").
				To(cashTokenContract).
				From(alice.id).
				Function("transfer").
				Inputs(map[string]any{
					"to":     bondCustodian.id,
					"amount": 1000,
					"data":   "0x",
				}).
				Prepare().
				Wait(5 * time.Second)
			testLog("Cash transfer to bond custodian prepared TX ID: %s", tx.ID())
			Expect(tx.Error()).To(BeNil())
			preparedPaymentTransfer = tx.PreparedTransaction()
			Expect(preparedPaymentTransfer).ToNot(BeNil())
			Expect(preparedPaymentTransfer.Transaction.Domain).To(BeEmpty( /* e.g. public */ ))
		})
	})
})
