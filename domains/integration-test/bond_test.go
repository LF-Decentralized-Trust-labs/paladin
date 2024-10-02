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

package integrationtest

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/integration-test/helpers"
	"github.com/kaleido-io/paladin/domains/noto/pkg/types"
	"github.com/kaleido-io/paladin/toolkit/pkg/algorithms"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	cashIssuer = "cash-issuer"
	bondIssuer = "bond-issuer"
)

func TestBond(t *testing.T) {
	ctx := context.Background()
	log.L(ctx).Infof("TestBond")
	domainName := "noto_" + tktypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)

	log.L(ctx).Infof("Deploying factories")
	contractSource := map[string][]byte{
		"noto": helpers.NotoFactoryJSON,
		"atom": helpers.AtomFactoryJSON,
	}
	contracts := deployContracts(ctx, t, notary, contractSource)
	for name, address := range contracts {
		log.L(ctx).Infof("%s deployed to %s", name, address)
	}

	_, notoTestbed := newNotoDomain(t, &types.DomainConfig{
		FactoryAddress: contracts["noto"],
	})
	done, tb, rpc := newTestbed(t, map[string]*testbed.TestbedDomain{
		domainName: notoTestbed,
	})
	defer done()

	_, aliceKey, err := tb.Components().KeyManager().ResolveKey(ctx, alice, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)
	_, bobKey, err := tb.Components().KeyManager().ResolveKey(ctx, bob, algorithms.ECDSA_SECP256K1_PLAINBYTES)
	require.NoError(t, err)

	atomFactory := helpers.InitAtom(t, tb, rpc, contracts["atom"])

	log.L(ctx).Infof("Deploying Noto")
	notoCash := helpers.DeployNoto(ctx, t, rpc, domainName, cashIssuer)
	notoBond := helpers.DeployNoto(ctx, t, rpc, domainName, bondIssuer)
	log.L(ctx).Infof("Noto cash deployed to %s", notoCash.Address)
	log.L(ctx).Infof("Noto bond deployed to %s", notoBond.Address)

	log.L(ctx).Infof("Initial mint: 100 cash to bond issuer/Alice/Bob, 100 bonds to bond issuer")
	notoCash.Mint(ctx, bondIssuer, 100).SignAndSend(cashIssuer).Wait()
	notoCash.Mint(ctx, alice, 100).SignAndSend(cashIssuer).Wait()
	notoCash.Mint(ctx, bob, 100).SignAndSend(cashIssuer).Wait()
	notoBond.Mint(ctx, bondIssuer, 100).SignAndSend(bondIssuer).Wait()

	bondDetails := map[string]any{
		"issueDate":          time.Now().Add(-24 * 300 * time.Hour).Unix(),
		"termLength":         5,
		"faceValue":          1000,
		"couponToken":        notoCash.Address,
		"couponRate":         5,
		"couponRateDecimals": 0,
		"numPayments":        10,
	}

	// TODO: this should be a Pente private contract, instead of a base ledger contract
	log.L(ctx).Infof("Issue a new bond to Alice")
	aliceBond := helpers.DeployBond(ctx, t, tb, bondIssuer, aliceKey, bondDetails)
	aliceBondIssue := aliceBond.Issue(ctx, 1).Prepare()

	notoBondIssue := notoBond.TransferWithApproval(ctx, alice, 1).Prepare(bondIssuer)
	notoCashTransfer := notoCash.TransferWithApproval(ctx, bondIssuer, 100).Prepare(alice)

	// TODO: disclose unmasked states

	issueAtom := atomFactory.Create(ctx, bondIssuer, []*helpers.AtomOperation{
		{
			ContractAddress: aliceBond.Address,
			CallData:        aliceBondIssue,
		},
		{
			ContractAddress: notoBond.Address,
			CallData:        notoBondIssue.EncodedCall,
		},
		{
			ContractAddress: notoCash.Address,
			CallData:        notoCashTransfer.EncodedCall,
		},
	})

	// TODO: all parties should verify the Atom

	aliceBond.Approve(ctx, issueAtom.Address).SignAndSend(bondIssuer).Wait()
	notoBond.ApproveTransfer(ctx, issueAtom.Address, notoBondIssue.EncodedCall).SignAndSend(bondIssuer).Wait()
	notoCash.ApproveTransfer(ctx, issueAtom.Address, notoCashTransfer.EncodedCall).SignAndSend(alice).Wait()
	issueAtom.Execute(ctx).SignAndSend(bondIssuer).Wait()

	couponPayment := aliceBond.CouponPayment(ctx)
	assert.Equal(t, uint64(25), couponPayment)
	claimedPayments := aliceBond.ClaimedPayments(ctx)
	assert.Equal(t, uint64(0), claimedPayments)
	availablePayments := aliceBond.AvailablePayments(ctx)
	assert.Equal(t, uint64(1), availablePayments)

	log.L(ctx).Infof("Claim coupon payment to Alice")
	notoCashPayment := notoCash.TransferWithApproval(ctx, alice, couponPayment).Prepare(bondIssuer)
	aliceBondPayment := aliceBond.ClaimPayment(ctx).Prepare()

	// TODO: disclose unmasked states

	paymentAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: notoCash.Address,
			CallData:        notoCashPayment.EncodedCall,
		},
		{
			ContractAddress: aliceBond.Address,
			CallData:        aliceBondPayment,
		},
		// TODO: need to verify that Alice owns the bond token on Noto
	})

	// TODO: all parties should verify the Atom

	notoCash.ApproveTransfer(ctx, paymentAtom.Address, notoCashPayment.EncodedCall).SignAndSend(bondIssuer).Wait()
	aliceBond.Approve(ctx, paymentAtom.Address).SignAndSend(alice).Wait()
	paymentAtom.Execute(ctx).SignAndSend(alice).Wait()

	claimedPayments = aliceBond.ClaimedPayments(ctx)
	assert.Equal(t, uint64(1), claimedPayments)
	availablePayments = aliceBond.AvailablePayments(ctx)
	assert.Equal(t, uint64(0), availablePayments)

	log.L(ctx).Infof("Alice sells bond to Bob")
	bobBond := helpers.DeployBond(ctx, t, tb, bondIssuer, bobKey, bondDetails)
	aliceBondTransfer := aliceBond.Transfer(ctx, 1).Prepare()
	bobBondIssue := bobBond.RecordTransfer(ctx, 1, claimedPayments).Prepare()
	notoBondTransfer := notoBond.TransferWithApproval(ctx, bob, 1).Prepare(alice)
	notoCashTransfer = notoCash.TransferWithApproval(ctx, alice, 100).Prepare(bob)

	// TODO: disclose unmasked states

	transferAtom := atomFactory.Create(ctx, alice, []*helpers.AtomOperation{
		{
			ContractAddress: aliceBond.Address,
			CallData:        aliceBondTransfer,
		},
		{
			ContractAddress: bobBond.Address,
			CallData:        bobBondIssue,
		},
		{
			ContractAddress: notoBond.Address,
			CallData:        notoBondTransfer.EncodedCall,
		},
		{
			ContractAddress: notoCash.Address,
			CallData:        notoCashTransfer.EncodedCall,
		},
	})

	// TODO: all parties should verify the Atom

	aliceBond.Approve(ctx, transferAtom.Address).SignAndSend(alice).Wait()
	bobBond.Approve(ctx, transferAtom.Address).SignAndSend(bondIssuer).Wait()
	notoBond.ApproveTransfer(ctx, transferAtom.Address, notoBondTransfer.EncodedCall).SignAndSend(alice).Wait()
	notoCash.ApproveTransfer(ctx, transferAtom.Address, notoCashTransfer.EncodedCall).SignAndSend(bob).Wait()
	transferAtom.Execute(ctx).SignAndSend(alice).Wait()
}
