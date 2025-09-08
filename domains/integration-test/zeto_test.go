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
	_ "embed"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/testbed"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/integration-test/helpers"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zeto"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/suite"
)

//go:embed helpers/abis/Zeto_Anon.json
var zetoAnonAbi []byte

var (
	controllerName = "controller"
)

// This is the path to the contracts file
// it should be set by the test runner
var contractsFile string

type zetoDomainTestSuite struct {
	suite.Suite
	hdWalletSeed      *testbed.UTInitFunction
	deployedContracts *helpers.ZetoDomainContracts
	domainName        string
	domain            zeto.Zeto
	rpc               rpcclient.Client
	tb                testbed.Testbed
	done              func()
}

func (s *zetoDomainTestSuite) SetupSuite() {
	log.SetLevel("debug")
	s.hdWalletSeed = testbed.HDWalletSeedScopedToTest()
	domainContracts := helpers.DeployZetoContracts(s.T(), s.hdWalletSeed, contractsFile, controllerName)
	s.deployedContracts = domainContracts
	ctx := context.Background()
	domainName := "zeto_" + pldtypes.RandHex(8)
	log.L(ctx).Infof("Domain name = %s", domainName)
	config := helpers.PrepareZetoConfig(s.T(), s.deployedContracts, "../zeto/zkp")
	waitForZeto, zetoTestbed := newZetoDomain(s.T(), config, domainContracts.FactoryAddress)
	done, _, tb, rpc := newTestbed(s.T(), s.hdWalletSeed, map[string]*testbed.TestbedDomain{
		domainName: zetoTestbed,
	})
	s.domainName = domainName
	s.domain = <-waitForZeto
	s.rpc = rpc
	s.tb = tb
	s.done = done
}

func (s *zetoDomainTestSuite) TearDownSuite() {
	s.done()
}
