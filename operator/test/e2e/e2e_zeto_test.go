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
	"fmt"
	"math/big"
	"time"

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

const tokenType = "Zeto_Anon"
const isNullifier = false

// const tokenType = "Zeto_AnonNullifier"
// const isNullifier = true

var zetoConstructorABI = &abi.Entry{
	Type: abi.Constructor, Inputs: abi.ParameterArray{
		{Name: "tokenName", Type: "string"},
	},
}

var _ = Describe(fmt.Sprintf("zeto - %s", tokenType), Ordered, func() {
	BeforeAll(func() {
		// Skip("for now")
	})

	AfterAll(func() {
	})

	Context("Zeto domain verification", func() {

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

		var zetoContract *tktypes.EthAddress
		operator := "zeto.operator@node1"
		It("deploys a zeto", func() {
			deploy := rpc["node1"].ForABI(ctx, abi.ABI{zetoConstructorABI}).
				Private().
				Domain("zeto").
				Constructor().
				From(operator).
				Inputs(&zetotypes.InitializerParams{
					TokenName: tokenType,
				}).
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			zetoContract = deploy.Receipt().ContractAddress
			testLog("Zeto (%s) contract %s deployed by TX %s", tokenType, zetoContract, deploy.ID())
		})

		var zetoCoinSchemaID *tktypes.Bytes32
		It("gets the coin schema", func() {
			var schemas []*pldapi.Schema
			err := rpc["node1"].CallRPC(ctx, &schemas, "pstate_listSchemas", "zeto")
			Expect(err).To(BeNil())
			for _, s := range schemas {
				if s.Signature == "type=ZetoCoin(uint256 salt,bytes32 owner,uint256 amount),labels=[owner,amount]" {
					zetoCoinSchemaID = &s.ID
				}
			}
			Expect(zetoCoinSchemaID).ToNot(BeNil())
		})

		logWallet := func(identity, node string) {
			var addr tktypes.HexBytes
			err := rpc[node].CallRPC(ctx, &addr, "ptx_resolveVerifier", identity, "domain:zeto:snark:babyjubjub", "iden3_pubkey_babyjubjub_compressed_0x")
			Expect(err).To(BeNil())
			method := "pstate_queryContractStates"
			if isNullifier {
				method = "pstate_queryContractNullifiers"
			}
			var coins []*zetotypes.ZetoCoinState
			err = rpc[node].CallRPC(ctx, &coins, method, "zeto", zetoContract, zetoCoinSchemaID,
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

		It("mints some zetos to bob on node1", func() {
			for _, amount := range []*tktypes.HexUint256{
				with10Decimals(15),
				with10Decimals(25), // 40
				with10Decimals(30), // 70
				with10Decimals(42), // 112
			} {
				txn := rpc["node1"].ForABI(ctx, zetotypes.ZetoABI).
					Private().
					Domain("zeto").
					Function("mint").
					To(zetoContract).
					From(operator).
					Inputs(&zetotypes.MintParams{
						Mints: []*zetotypes.TransferParamEntry{
							{
								To:     "bob@node1",
								Amount: amount,
							},
						},
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Zeto mint transaction %s", txn.ID())
				Expect(txn.Error()).To(BeNil())
				logWallet("bob", "node1")
			}
		})

		It("sends some zetos to sally on node2", func() {
			for _, amount := range []*tktypes.HexUint256{
				with10Decimals(33), // 79
				with10Decimals(66), // 13
			} {
				txn := rpc["node1"].ForABI(ctx, zetotypes.ZetoABI).
					Private().
					Domain("zeto").
					Function("transfer").
					To(zetoContract).
					From("bob@node1").
					Inputs(&zetotypes.TransferParams{
						Transfers: []*zetotypes.TransferParamEntry{
							{
								To:     "sally@node2",
								Amount: amount,
							},
						},
					}).
					Send().
					Wait(5 * time.Second)
				testLog("Zeto transfer transaction %s", txn.ID())
				Expect(txn.Error()).To(BeNil())
				logWallet("bob", "node1")
				logWallet("sally", "node2")
			}
		})

		It("sally on node2 sends some zetos to fred on node3", func() {
			txn := rpc["node2"].ForABI(ctx, zetotypes.ZetoABI).
				Private().
				Domain("zeto").
				Function("transfer").
				To(zetoContract).
				From("sally@node2").
				Inputs(&zetotypes.TransferParams{
					Transfers: []*zetotypes.TransferParamEntry{
						{
							To:     "fred@node3",
							Amount: with10Decimals(20),
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto transfer transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("sally", "node2")
			logWallet("fred", "node3")
			testLog("done testing zeto in isolation")
		})

	})
})
