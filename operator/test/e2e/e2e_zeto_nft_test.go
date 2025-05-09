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
	"time"

	_ "embed"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	zetotypes "github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/kaleido-io/paladin/toolkit/pkg/verifiers"
)

//go:embed abis/zeto/Zeto_NfAnon.json
var zetoNfAnonBuildJSON []byte

const tokenTypeNFT = constants.TOKEN_NF_ANON
const isNullifierNFT = false

var zetoNFTConstructorABI = &abi.Entry{
	Type: abi.Constructor,
	Inputs: abi.ParameterArray{
		{Name: "tokenName", Type: "string"},
	},
}

var _ = Describe(fmt.Sprintf("zeto - %s", tokenTypeNFT), Ordered, func() {
	BeforeAll(func() {
		// Optionally skip or setup global state if needed
		// Skip("for now")
	})

	AfterAll(func() {
		// Cleanup if necessary
	})

	Context("Zeto NFT domain verification", func() {
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
		It("deploys a zeto NFT contract", func() {
			deploy := rpc["node1"].ForABI(ctx, abi.ABI{zetoNFTConstructorABI}).
				Private().
				Domain("zeto").
				Constructor().
				From(operator).
				Inputs(&zetotypes.DeployParams{
					TokenName:     tokenTypeNFT,
					IsNonFungible: true,
					// InitialOwner:  operator,
				}).
				Send().
				Wait(5 * time.Second)
			Expect(deploy.Error()).To(BeNil())
			Expect(deploy.Receipt().ContractAddress).ToNot(BeNil())
			zetoContract = deploy.Receipt().ContractAddress
			testLog("Zeto (%s) contract %s deployed by TX %s", tokenTypeNFT, zetoContract, deploy.ID())
		})

		var zetoNFTSchemaID *tktypes.Bytes32
		It("gets the NFT schema", func() {
			var schemas []*pldapi.Schema
			err := rpc["node1"].CallRPC(ctx, &schemas, "pstate_listSchemas", "zeto")
			Expect(err).To(BeNil())
			for _, s := range schemas {
				if s.Signature == "type=ZetoNFToken(uint256 salt,string uri,bytes32 owner,uint256 tokenID),labels=[owner,tokenID]" {
					zetoNFTSchemaID = &s.ID
				}
			}
			Expect(zetoNFTSchemaID).ToNot(BeNil())
		})

		logWallet := func(identity, node string) {
			var addr tktypes.Bytes32
			err := rpc[node].CallRPC(ctx, &addr, "ptx_resolveVerifier", identity, "domain:zeto:snark:babyjubjub", "iden3_pubkey_babyjubjub_compressed_0x")
			Expect(err).To(BeNil())
			method := "pstate_queryContractStates"
			if isNullifierNFT {
				method = "pstate_queryContractNullifiers"
			}
			var nfts []*zetotypes.ZetoNFTState
			err = rpc[node].CallRPC(ctx, &nfts, method, "zeto", zetoContract, zetoNFTSchemaID,
				query.NewQueryBuilder().Equal("owner", addr).Limit(100).Query(),
				"available")
			Expect(err).To(BeNil())
			summary := make([]string, len(nfts))
			for i, nft := range nfts {
				summary[i] = fmt.Sprintf("%s...[tokenID=%s, uri=%s]", nft.ID.String()[0:8], nft.Data.TokenID.Int().Text(10), nft.Data.URI)
			}
			testLog("%s@%s owns %d NFTs: %v", identity, node, len(nfts), summary)
		}

		It("mints two NFTs to bob on node1", func() {
			txn := rpc["node1"].ForABI(ctx, zetotypes.ZetoNonFungibleABI).
				Private().
				Domain("zeto").
				Function("mint").
				To(zetoContract).
				From(operator).
				Inputs(&zetotypes.NonFungibleMintParams{
					Mints: []*zetotypes.NonFungibleTransferParamEntry{
						{To: "bob@node1", URI: "https://example.com/token/name1"},
						{To: "bob@node1", URI: "https://example.com/token/name2"},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto NFT mint transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("bob", "node1")
		})

		var bobNFTs []*zetotypes.ZetoNFTState
		It("sends one NFT to sally on node2", func() {
			// Fetch Bob's NFTs to get a tokenID
			var addr tktypes.Bytes32
			err := rpc["node1"].CallRPC(ctx, &addr, "ptx_resolveVerifier", "bob@node1", "domain:zeto:snark:babyjubjub", "iden3_pubkey_babyjubjub_compressed_0x")
			Expect(err).To(BeNil())
			err = rpc["node1"].CallRPC(ctx, &bobNFTs, "pstate_queryContractStates", "zeto", zetoContract, zetoNFTSchemaID,
				query.NewQueryBuilder().Equal("owner", addr).Limit(100).Query(),
				"available")
			Expect(err).To(BeNil())
			Expect(bobNFTs).To(HaveLen(2))

			txn := rpc["node1"].ForABI(ctx, zetotypes.ZetoNonFungibleABI).
				Private().
				Domain("zeto").
				Function("transfer").
				To(zetoContract).
				From("bob@node1").
				Inputs(&zetotypes.NonFungibleTransferParams{
					Transfers: []*zetotypes.NonFungibleTransferParamEntry{
						{
							To:      "sally@node2",
							TokenID: bobNFTs[0].Data.TokenID,
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto NFT transfer transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("bob", "node1")
			logWallet("sally", "node2")
		})

		It("sally on node2 sends the NFT to fred on node3", func() {
			txn := rpc["node2"].ForABI(ctx, zetotypes.ZetoNonFungibleABI).
				Private().
				Domain("zeto").
				Function("transfer").
				To(zetoContract).
				From("sally@node2").
				Inputs(&zetotypes.NonFungibleTransferParams{
					Transfers: []*zetotypes.NonFungibleTransferParamEntry{
						{
							To:      "fred@node3",
							TokenID: bobNFTs[0].Data.TokenID, // Same tokenID from Bob's first NFT
						},
					},
				}).
				Send().
				Wait(5 * time.Second)
			testLog("Zeto NFT transfer transaction %s", txn.ID())
			Expect(txn.Error()).To(BeNil())
			logWallet("sally", "node2")
			logWallet("fred", "node3")
			testLog("done testing zeto NFT in isolation")
		})
	})
})
