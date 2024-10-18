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
	"time"

	_ "embed"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/kaleido-io/paladin/operator/test/utils"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
)

//go:embed abis/PenteFactory.json
var penteFactoryBuild string

//go:embed abis/NotoFactory.json
var notoFactoryBuild string

var _ = Describe("controller", Ordered, func() {
	BeforeAll(func() {
	})

	AfterAll(func() {
	})

	Context("Paladin Single Node", func() {
		It("start up the node", func() {
			ctx := context.Background()

			rpc, err := rpcclient.NewHTTPClient(ctx, &pldconf.HTTPClientConfig{
				URL: "http://127.0.0.1:31548",
			})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("waiting for Paladin node to be ready") // TODO: We should have the paladin pod ready once this is ready
			EventuallyWithOffset(1, func() error {
				var txs []*pldapi.Transaction
				return rpc.CallRPC(ctx, &txs, "ptx_queryPendingTransactions", query.NewQueryBuilder().Limit(1).Query(), false)
			}, 5*time.Minute, 5*time.Second).Should(Succeed())

			deployer := utils.TestDeployer{RPC: rpc, From: "deployerKey"}

			By("deploying the pente factory")
			_, err = deployer.DeploySmartContractDeploymentBytecode(ctx, penteFactoryBuild, []any{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			// penteFactoryAddr := receipt.ContractAddress
			// By("recording pente factory deployed at " + penteFactoryAddr.String())

			By("deploying the noto factory")
			_, err = deployer.DeploySmartContractDeploymentBytecode(ctx, notoFactoryBuild, []any{})
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			// notoFactoryAddr := receipt.ContractAddress
			// By("recording noto factory deployed at " + notoFactoryAddr.String())

		})
	})
})
