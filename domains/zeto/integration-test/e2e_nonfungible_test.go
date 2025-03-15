package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kaleido-io/paladin/core/pkg/testbed"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/constants"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/types"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zeto"
	"github.com/kaleido-io/paladin/domains/zeto/pkg/zetosigner/zetosignerapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/log"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestNonFungibleZetoDomainTestSuite(t *testing.T) {
	contractsFile = "./config-for-deploy-non-fungible.yaml"
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
	var zetoAddress tktypes.EthAddress
	rpcerr := s.rpc.CallRPC(ctx, &zetoAddress, "testbed_deploy",
		s.domainName, controllerName, &types.InitializerParams{
			TokenName: tokenName,
		})
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}

	log.L(ctx).Infof("Zeto instance deployed to %s", zetoAddress)

	log.L(ctx).Info("*************************************")
	log.L(ctx).Infof("Mint two UTXOs to controller")
	log.L(ctx).Info("*************************************")

	uris := []string{
		"https://example.com/token/name1",
		"https://example.com/token/name2",
	}
	_, err := s.mint(ctx, zetoAddress, controllerName, uris)
	require.NoError(t, err)

	var controllerAddr tktypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &controllerAddr, "ptx_resolveVerifier", controllerName, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the controller has the two UTXOs
	controllerNFTs := findAvailableTokens(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken, &controllerAddr)
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
	_, err = s.transfer(ctx, zetoAddress, controllerName, recipient1Name, controllerNFTs[0].Data.TokenID)
	require.NoError(t, err)

	// get recipient1 address
	var recipient1Addr tktypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &recipient1Addr, "ptx_resolveVerifier", recipient1Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the recipient1 has the UTXO
	recipient1NFTs := findAvailableTokens(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken, &recipient1Addr)
	require.Len(t, recipient1NFTs, 1)
	assert.Equal(t, recipient1Addr.String(), recipient1NFTs[0].Data.Owner.String())
	assert.Equal(t, uris[0], recipient1NFTs[0].Data.URI)
	assert.False(t, recipient1NFTs[0].Data.Salt.NilOrZero())
	assert.False(t, recipient1NFTs[0].Data.TokenID.NilOrZero())
	assert.False(t, recipient1NFTs[0].ID.NilOrZero())
	assert.False(t, recipient1NFTs[0].ContractAddress.IsZero())

	// confirm that the controller has the second UTXO
	controllerNFTs = findAvailableTokens(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken, &controllerAddr)
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
	_, err = s.transfer(ctx, zetoAddress, recipient1Name, recipient2Name, recipient1NFTs[0].Data.TokenID)
	require.NoError(t, err)

	// get recipient2 address
	var recipient2Addr tktypes.Bytes32
	rpcerr = s.rpc.CallRPC(ctx, &recipient2Addr, "ptx_resolveVerifier", recipient2Name, zetosignerapi.AlgoDomainZetoSnarkBJJ(s.domainName), zetosignerapi.IDEN3_PUBKEY_BABYJUBJUB_COMPRESSED_0X)
	require.Nil(t, rpcerr)

	// confirm that the recipient2 has the UTXO
	recipient2NFTs := findAvailableTokens(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken, &recipient2Addr)
	require.Len(t, recipient2NFTs, 1)
	assert.Equal(t, recipient2Addr.String(), recipient2NFTs[0].Data.Owner.String())
	assert.Equal(t, uris[0], recipient2NFTs[0].Data.URI)
	assert.False(t, recipient2NFTs[0].Data.Salt.NilOrZero())
	assert.False(t, recipient2NFTs[0].Data.TokenID.NilOrZero())
	assert.False(t, recipient2NFTs[0].ID.NilOrZero())
	assert.False(t, recipient2NFTs[0].ContractAddress.IsZero())

	// confirm that the recipient1 has no UTXOs
	recipient1NFTs = findAvailableTokens(t, ctx, s.rpc, s.domain, zetoAddress, nil, isNullifiersToken, &recipient1Addr)
	require.Len(t, recipient1NFTs, 0)
}

func (s *nonFungibleTestSuiteHelper) mint(ctx context.Context, zetoAddress tktypes.EthAddress, minter string, URIs []string) (invokeResult *testbed.TransactionResult, err error) {
	var params []*types.NonFungibleTransferParamEntry
	for _, uri := range URIs {
		params = append(params, &types.NonFungibleTransferParamEntry{
			To:  minter,
			URI: uri,
		})
	}
	mintParam := types.NonFungibleMintParams{
		Mints: params,
	}
	paramsJson, err := json.Marshal(&mintParam)
	if err != nil {
		return nil, err
	}
	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     minter,
			To:       &zetoAddress,
			Function: "mint",
			Data:     paramsJson,
		},
		ABI: types.ZetoNonFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return invokeResult, nil
}

func (s *nonFungibleTestSuiteHelper) transfer(ctx context.Context, zetoAddress tktypes.EthAddress, sender, receiver string, tokenID *tktypes.HexUint256) (*testbed.TransactionResult, error) {
	var invokeResult testbed.TransactionResult
	params := &types.NonFungibleTransferParamEntry{
		To:      receiver,
		TokenID: tokenID,
	}

	transferParams := types.NonFungibleTransferParams{
		Transfers: []*types.NonFungibleTransferParamEntry{params},
	}
	paramsJson, err := json.Marshal(&transferParams)
	if err != nil {
		return nil, err
	}

	rpcerr := s.rpc.CallRPC(ctx, &invokeResult, "testbed_invoke", &pldapi.TransactionInput{
		TransactionBase: pldapi.TransactionBase{
			From:     sender,
			To:       &zetoAddress,
			Function: "transfer",
			Data:     paramsJson,
		},
		ABI: types.ZetoNonFungibleABI,
	}, true)
	if rpcerr != nil {
		return nil, rpcerr.RPCError()
	}
	return &invokeResult, nil
}
func findAvailableTokens(t *testing.T, ctx context.Context, rpc rpcclient.Client, zeto zeto.Zeto, address tktypes.EthAddress, jq *query.QueryJSON, useNullifiers bool, owner *tktypes.Bytes32) []*types.ZetoNFTState {
	if jq == nil {
		jq = query.NewQueryBuilder().
			Limit(100).
			Query()
	}
	methodName := "pstate_queryContractStates"
	if useNullifiers {
		methodName = "pstate_queryContractNullifiers"
	}
	var nfts []*types.ZetoNFTState
	rpcerr := rpc.CallRPC(ctx, &nfts, methodName,
		zeto.Name(),
		address,
		zeto.NFTSchemaID(),
		jq,
		"available")
	if rpcerr != nil {
		require.NoError(t, rpcerr.RPCError())
	}

	// Filter out the tokens that are not owned by the owner
	if owner != nil {
		var filteredNfts []*types.ZetoNFTState
		for _, nft := range nfts {
			if nft.Data.Owner.String() == owner.String() {
				filteredNfts = append(filteredNfts, nft)
			}
		}
		nfts = filteredNfts
	}

	return nfts
}
