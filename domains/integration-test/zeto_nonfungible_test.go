package integrationtest

import (
	"context"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/integration-test/helpers"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/constants"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/types"
	"github.com/LF-Decentralized-Trust-labs/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestNonFungibleZetoDomainTestSuite(t *testing.T) {
	contractsFile = "./zeto/config-for-deploy-non-fungible.yaml"
	suite.Run(t, new(nonFungibleTestSuiteHelper))
}

type nonFungibleTestSuiteHelper struct {
	zetoDomainTestSuite
}

func (s *nonFungibleTestSuiteHelper) TestZeto_NfAnon() {
	s.testZeto(s.T(), constants.TOKEN_NF_ANON, false)
}

func (s *nonFungibleTestSuiteHelper) testZeto(t *testing.T, tokenName string, isNullifiersToken bool) {
	ctx := context.Background()
	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Deploying an instance of the %s token", tokenName)
	log.L(ctx).Info("*************************************")
	s.setupContractsAbi(t, ctx, tokenName)
	zeto := helpers.DeployZetoNonFungible(ctx, t, s.rpc, s.domainName, controllerName, tokenName)
	zetoAddress := zeto.Address
	log.L(ctx).Infof("Zeto instance deployed to %s", zeto.Address)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Mint two UTXOs to controller")
	log.L(ctx).Info("*************************************")

	uris := []string{
		"https://example.com/token/name1",
		"https://example.com/token/name2",
	}
	zeto.Mint(ctx, []string{controllerName, controllerName}, uris).SignAndSend(controllerName, true).Wait()

	var controllerAddr pldtypes.Bytes32
	rpcerr := s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the controller has the two UTXOs
	controllerNFTs := findAvailableNFTs(t, ctx, s.rpc, s.domain.Name(), s.domain.NFTSchemaID(), zetoAddress, nil, isNullifiersToken, &controllerAddr)
	controllerNFTs = filterNFTs(controllerNFTs, &controllerAddr)
	require.Len(t, controllerNFTs, len(uris))
	for i := range controllerNFTs {
		assert.Equal(t, controllerAddr.String(), controllerNFTs[i].Data.Owner.String())
		assert.Equal(t, uris[i], controllerNFTs[i].Data.URI)
		assert.False(t, controllerNFTs[i].Data.Salt.NilOrZero())
		assert.False(t, controllerNFTs[i].Data.TokenID.NilOrZero())
		assert.False(t, controllerNFTs[i].ID.NilOrZero())
		assert.False(t, controllerNFTs[i].ContractAddress.IsZero())
	}

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Transfer UTXO from the controller to recipient1")
	log.L(ctx).Info("*************************************")

	// transfer the first UTXO to recipient1
	zeto.Transfer(ctx, recipient1Name, controllerNFTs[0].Data.TokenID).SignAndSend(controllerName, true).Wait()

	// get recipient1 address
	var recipient1Addr pldtypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &recipient1Addr, "ptx_resolveVerifier", recipient1Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the recipient1 has the UTXO
	recipient1NFTs := findAvailableNFTs(t, ctx, s.rpc, s.domain.Name(), s.domain.NFTSchemaID(), zetoAddress, nil, isNullifiersToken, &recipient1Addr)
	recipient1NFTs = filterNFTs(recipient1NFTs, &recipient1Addr)
	require.Len(t, recipient1NFTs, 1)
	assert.Equal(t, recipient1Addr.String(), recipient1NFTs[0].Data.Owner.String())
	assert.Equal(t, uris[0], recipient1NFTs[0].Data.URI)
	assert.False(t, recipient1NFTs[0].Data.Salt.NilOrZero())
	assert.False(t, recipient1NFTs[0].Data.TokenID.NilOrZero())
	assert.False(t, recipient1NFTs[0].ID.NilOrZero())
	assert.False(t, recipient1NFTs[0].ContractAddress.IsZero())

	// confirm that the controller has the second UTXO
	controllerNFTs = findAvailableNFTs(t, ctx, s.rpc, s.domain.Name(), s.domain.NFTSchemaID(), zetoAddress, nil, isNullifiersToken, &controllerAddr)
	controllerNFTs = filterNFTs(controllerNFTs, &controllerAddr)
	require.Len(t, controllerNFTs, 1)
	assert.Equal(t, controllerAddr.String(), controllerNFTs[0].Data.Owner.String())
	assert.Equal(t, uris[1], controllerNFTs[0].Data.URI)
	assert.False(t, controllerNFTs[0].Data.Salt.NilOrZero())
	assert.False(t, controllerNFTs[0].Data.TokenID.NilOrZero())
	assert.False(t, controllerNFTs[0].ID.NilOrZero())
	assert.False(t, controllerNFTs[0].ContractAddress.IsZero())

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Transfer UTXO from the recipient1 to recipient2")
	log.L(ctx).Info("*************************************")

	// transfer the UTXO from recipient1 to recipient2
	zeto.Transfer(ctx, recipient2Name, recipient1NFTs[0].Data.TokenID).SignAndSend(recipient1Name, true).Wait()

	// get recipient2 address
	var recipient2Addr pldtypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &recipient2Addr, "ptx_resolveVerifier", recipient2Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the recipient2 has the UTXO
	recipient2NFTs := findAvailableNFTs(t, ctx, s.rpc, s.domain.Name(), s.domain.NFTSchemaID(), zetoAddress, nil, isNullifiersToken, &recipient2Addr)
	recipient2NFTs = filterNFTs(recipient2NFTs, &recipient2Addr)
	require.Len(t, recipient2NFTs, 1)
	assert.Equal(t, recipient2Addr.String(), recipient2NFTs[0].Data.Owner.String())
	assert.Equal(t, uris[0], recipient2NFTs[0].Data.URI)
	assert.False(t, recipient2NFTs[0].Data.Salt.NilOrZero())
	assert.False(t, recipient2NFTs[0].Data.TokenID.NilOrZero())
	assert.False(t, recipient2NFTs[0].ID.NilOrZero())
	assert.False(t, recipient2NFTs[0].ContractAddress.IsZero())

	// confirm that the recipient1 has no UTXOs
	recipient1NFTs = findAvailableNFTs(t, ctx, s.rpc, s.domain.Name(), s.domain.NFTSchemaID(), zetoAddress, nil, isNullifiersToken, &recipient1Addr)
	recipient1NFTs = filterNFTs(recipient1NFTs, &recipient1Addr)
	require.Len(t, recipient1NFTs, 0)
}

func findAvailableNFTs(t *testing.T, ctx context.Context, rpc rpcclient.Client, domainName, domainSchemaId string, address *pldtypes.EthAddress, jq *query.QueryJSON, useNullifiers bool, owner *pldtypes.Bytes32) []*types.ZetoNFTState {
	methodName := "pstate_queryContractStates"
	if useNullifiers {
		methodName = "pstate_queryContractNullifiers"
	}
	nfts := findAvailableCoins(t, ctx, rpc, domainName, domainSchemaId, methodName, address, jq, func(nfts []*types.ZetoNFTState) bool {
		return len(nfts) > 0
	})

	return nfts
}

func filterNFTs(nfts []*types.ZetoNFTState, owner *pldtypes.Bytes32) []*types.ZetoNFTState {
	// Filter out the tokens that are not owned by the owner
	if owner != nil {
		var filteredNfts []*types.ZetoNFTState
		for _, nft := range nfts {
			if nft.Data.Owner.String() == owner.String() {
				filteredNfts = append(filteredNfts, nft)
			}
		}
		return filteredNfts
	}
	return nfts
}
