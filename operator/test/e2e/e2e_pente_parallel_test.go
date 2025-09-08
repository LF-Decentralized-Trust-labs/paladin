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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	_ "embed"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	nototypes "github.com/LF-Decentralized-Trust-labs/paladin/domains/noto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/solutils"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/algorithms"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/verifiers"
)

var _ = Describe("pente - parallelism on a single contract", Ordered, func() {
	BeforeAll(func() {
		// Skip("for now")
	})

	AfterAll(func() {
	})

	Context("Pente with an ERC-20 modifying multiple accounts concurrently contending for account state", func() {

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
			connectNode(node1HttpURL, paladinPrefix+"1")
			connectNode(node2HttpURL, paladinPrefix+"2")
			connectNode(node3HttpURL, paladinPrefix+"3")
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

		penteGroupStars := nototypes.PentePrivateGroup{
			Salt:    pldtypes.RandBytes32(),                                                                                                             // unique salt must be shared privately to retain anonymity
			Members: []string{fmt.Sprintf("tara@%s1", paladinPrefix), fmt.Sprintf("hoshi@%s2", paladinPrefix), fmt.Sprintf("seren@%s3", paladinPrefix)}, // these will be salted to establish the endorsement key identifiers
		}

		var penteContract *pldtypes.EthAddress
		It("deploys a pente privacy group across all three nodes", func() {

			const ENDORSEMENT_TYPE__GROUP_SCOPED_IDENTITIES = "group_scoped_identities"

			deploy := rpc[paladinPrefix+"1"].ForABI(ctx, abi.ABI{penteConstructorABI}).
				Private().
				Domain("pente").
				Constructor().
				Inputs(&penteConstructorParams{
					Group:                penteGroupStars,
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
			testLog("Pente privacy group %s (salt=%s) deployed by TX %s", penteContract, penteGroupStars.Salt, deploy.ID())
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
					Group:    penteGroupStars,
					Bytecode: erc20Simple.Bytecode,
					Inputs: map[string]any{
						"name":   "Stars",
						"symbol": "STAR",
					},
				}).
				From(fmt.Sprintf("tara@%s1", paladinPrefix)).
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

		getEthAddress := func(identity, node string) pldtypes.EthAddress {
			addr, err := rpc[node].PTX().ResolveVerifier(ctx, fmt.Sprintf("%s@%s", identity, node), algorithms.ECDSA_SECP256K1, verifiers.ETH_ADDRESS)
			Expect(err).To(BeNil())
			return *pldtypes.MustEthAddress(addr)
		}
		getERC20Balance := func(identity, node string) *pldtypes.HexUint256 {
			addr := getEthAddress(identity, node)
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
					Group: penteGroupStars,
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

		users := [][]string{
			{"tara", paladinPrefix + "1"},
			{"hoshi", paladinPrefix + "2"},
			{"seren", paladinPrefix + "3"},
		}

		It("mints some ERC-20 inside the the privacy group", func() {

			for _, user := range users {

				invoke := rpc[paladinPrefix+"1"].ForABI(ctx, erc20PrivateABI).
					Private().
					Domain("pente").
					To(penteContract).
					Function("mint").
					Inputs(&penteInvokeParams{
						Group: penteGroupStars,
						To:    *erc20StarsAddr,
						Inputs: map[string]any{
							"to":     getEthAddress(user[0], user[1]),
							"amount": with18Decimals(1000),
						},
					}).
					From(fmt.Sprintf("tara@%s1", paladinPrefix)). // operator
					Send().
					Wait(5 * time.Second)
				testLog("SimpleERC20 mint transaction %s", invoke.ID())
				Expect(invoke.Error()).To(BeNil())
			}

		})

		startingBalance := int64(1000)
		It("check ERC-20 balance of each", func() {

			for _, user := range users {
				userBalance := getERC20Balance(user[0], user[1])
				testLog("SimpleERC20 balance after mint to %s@%s: %s", user[0], user[1], userBalance.Int())
				Expect(userBalance.String()).To(Equal(with18Decimals(startingBalance).String()))
			}

		})

		It("runs three parallel sets of transfers, with each parallel set being synchronous", func() {

			results := make(chan error)
			for _iUser, _user := range users {
				go func(iUser int, user []string) {
					var err error
					defer func() {
						results <- err
					}()

					const count = 10
					toUser := users[(iUser+1)%len(users)]
					for i := 0; i < count && err == nil; i++ {
						bigAmount, _ := rand.Int(rand.Reader, big.NewInt(9))
						amount := bigAmount.Int64() + 1
						invoke := rpc[user[1]].ForABI(ctx, erc20PrivateABI).
							Private().
							Domain("pente").
							To(penteContract).
							Function("transfer").
							Inputs(&penteInvokeParams{
								Group: penteGroupStars,
								To:    *erc20StarsAddr,
								Inputs: map[string]any{
									"to":    getEthAddress(toUser[0], toUser[1]),
									"value": with18Decimals(amount),
								},
							}).
							From(fmt.Sprintf("%s@%s", user[0], user[1])).
							Send().
							// We submit the transactions one-at-a-time within each go-routine in this test
							// (but have three concurrent go routines running)
							Wait(15 * time.Second)
						testLog("[%d]:%.3d/%.3d SimpleERC20 mint %d from %s@%s to %s@%s transaction %s",
							iUser, i, count, amount, user[0], user[1], toUser[0], toUser[1], invoke.ID())
						err = invoke.Error()
					}
				}(_iUser, _user)
			}
			// Wait for the three go routines to complete
			for i := 0; i < len(users); i++ {
				Expect(<-results).To(BeNil())
			}
		})

		It("check ERC-20 balances add up to the correct total", func() {

			totalBalance := new(big.Int)
			for _, user := range users {
				userBalance := getERC20Balance(user[0], user[1])
				testLog("SimpleERC20 balance %s@%s after transfers: %s", user[0], user[1], userBalance.Int())
				totalBalance = totalBalance.Add(totalBalance, userBalance.Int())
			}
			Expect(totalBalance.String()).To(Equal(with18Decimals(startingBalance * int64(len(users))).Int().String()))

		})

		It("runs three parallel sets of transfers, all submitted as a stream and checked at the end", func() {

			results := make(chan []pldclient.SentTransaction)
			for _iUser, _user := range users {
				go func(iUser int, user []string) {
					const count = 10
					transfers := make([]pldclient.SentTransaction, 0, count)
					toUser := users[(iUser+1)%len(users)]
					for i := 0; i < count; i++ {
						bigAmount, _ := rand.Int(rand.Reader, big.NewInt(9))
						amount := bigAmount.Int64() + 1
						invoke := rpc[user[1]].ForABI(ctx, erc20PrivateABI).
							Private().
							Domain("pente").
							To(penteContract).
							Function("transfer").
							Inputs(&penteInvokeParams{
								Group: penteGroupStars,
								To:    *erc20StarsAddr,
								Inputs: map[string]any{
									"to":    getEthAddress(toUser[0], toUser[1]),
									"value": with18Decimals(amount),
								},
							}).
							From(fmt.Sprintf("%s@%s", user[0], user[1])).
							Send()
						testLog("[%d]:%.3d/%.3d SimpleERC20 mint %d from %s@%s to %s@%s transaction %s",
							iUser, i, count, amount, user[0], user[1], toUser[0], toUser[1], invoke.ID())
						transfers = append(transfers, invoke)
					}
					results <- transfers
				}(_iUser, _user)
			}
			// Wait for the three go routines to complete
			for i := 0; i < len(users); i++ {
				transfers := <-results
				for _, transfer := range transfers {
					testLog("SimpleERC20 wait for completion of transfer %s", transfer.ID())
					Expect(transfer.Wait(10 * time.Second).Error()).To(BeNil())
				}
			}
		})

		It("check ERC-20 balances add up to the correct total", func() {

			totalBalance := new(big.Int)
			for _, user := range users {
				userBalance := getERC20Balance(user[0], user[1])
				testLog("SimpleERC20 balance %s@%s after transfers: %s", user[0], user[1], userBalance.Int())
				totalBalance = totalBalance.Add(totalBalance, userBalance.Int())
			}
			Expect(totalBalance.String()).To(Equal(with18Decimals(startingBalance * int64(len(users))).Int().String()))

		})

	})
})
