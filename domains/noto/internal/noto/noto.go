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

package noto

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/domains/noto/internal/msgs"
	"github.com/LFDT-Paladin/paladin/domains/noto/pkg/types"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldapi"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/pldtypes"
	"github.com/LFDT-Paladin/paladin/sdk/go/pkg/solutils"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/algorithms"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/plugintk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/prototk"
	"github.com/LFDT-Paladin/paladin/toolkit/pkg/verifiers"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
)

// ParamValidator defines the interface for validating transaction parameters
type ParamValidator interface {
	ValidateParams(ctx context.Context, domainConfig *types.NotoParsedConfig, paramsJson string) (any, error)
}

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte

//go:embed abis/NotoFactory_V0.json
var notoFactoryV0JSON []byte

//go:embed abis/INoto.json
var notoInterfaceJSON []byte

//go:embed abis/INoto_V0.json
var notoInterfaceV0JSON []byte

//go:embed abis/INotoErrors.json
var notoErrorsJSON []byte

//go:embed abis/INotoHooks.json
var notoHooksJSON []byte

var (
	factoryBuild     = solutils.MustLoadBuild(notoFactoryJSON)
	factoryV0Build   = solutils.MustLoadBuild(notoFactoryV0JSON)
	interfaceBuild   = solutils.MustLoadBuild(notoInterfaceJSON)
	interfaceV0Build = solutils.MustLoadBuild(notoInterfaceV0JSON)
	errorsBuild      = solutils.MustLoadBuild(notoErrorsJSON)
	hooksBuild       = solutils.MustLoadBuild(notoHooksJSON)
)

var (
	EventTransfer      = "Transfer"
	EventLockCreated   = "LockCreated"
	EventLockSpent     = "LockSpent"
	EventLockCancelled = "LockCancelled"
	EventLockUpdated   = "LockUpdated"
	EventLockDelegated = "LockDelegated"

	// Old variant 0 events
	EventNotoTransfer       = "NotoTransfer"
	EventNotoLock           = "NotoLock"
	EventNotoUnlock         = "NotoUnlock"
	EventNotoUnlockPrepared = "NotoUnlockPrepared"
	EventNotoLockDelegated  = "NotoLockDelegated"
)

var allEvents = []string{
	EventTransfer,
	EventLockCreated,
	EventLockSpent,
	EventLockCancelled,
	EventLockUpdated,
	EventLockDelegated,
}

var allEventsV0 = []string{
	EventNotoTransfer,
	EventNotoLock,
	EventNotoUnlock,
	EventNotoUnlockPrepared,
	EventNotoLockDelegated,
}

var allEventsJSON = mustBuildEventsJSON(interfaceBuild.ABI, interfaceV0Build.ABI, errorsBuild.ABI)
var eventSignatures = mustLoadEventSignatures(interfaceBuild.ABI, allEvents)
var eventSignaturesV0 = mustLoadEventSignatures(interfaceV0Build.ABI, allEventsV0)

var allSchemas = []*abi.Parameter{
	types.NotoCoinABI,
	types.NotoLockInfoABI,
	types.NotoLockedCoinABI,
	types.TransactionDataABI,
}

var schemasJSON = mustParseSchemas(allSchemas)

type Noto struct {
	Callbacks plugintk.DomainCallbacks

	name                 string
	config               types.DomainConfig
	chainID              int64
	fixedSigningIdentity string
	coinSchema           *prototk.StateSchema
	lockedCoinSchema     *prototk.StateSchema
	dataSchema           *prototk.StateSchema
	lockInfoSchema       *prototk.StateSchema
}

type NotoDeployParams struct {
	TransactionID string              `json:"transactionId"`
	Name          string              `json:"name"`
	Symbol        string              `json:"symbol"`
	Notary        pldtypes.EthAddress `json:"notary"`
	Data          pldtypes.HexBytes   `json:"data"`
}

type NotoMintParams struct {
	TxId    string            `json:"txId"`
	Outputs []string          `json:"outputs"`
	Proof   pldtypes.HexBytes `json:"proof"`
	Data    pldtypes.HexBytes `json:"data"`
}

type NotoTransferParams struct {
	TxId    string            `json:"txId"`
	Inputs  []string          `json:"inputs"`
	Outputs []string          `json:"outputs"`
	Proof   pldtypes.HexBytes `json:"proof"`
	Data    pldtypes.HexBytes `json:"data"`
}

type NotoBurnParams struct {
	TxId    string            `json:"txId"`
	Inputs  []string          `json:"inputs"`
	Outputs []string          `json:"outputs"`
	Proof   pldtypes.HexBytes `json:"proof"`
	Data    pldtypes.HexBytes `json:"data"`
}

type CreateLockParams struct {
	Params NotoCreateLockParams `json:"params"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type NotoCreateLockParams struct {
	TxId          string            `json:"txId"`
	Inputs        []string          `json:"inputs"`
	Outputs       []string          `json:"outputs"`
	LockedOutputs []string          `json:"lockedOutputs"`
	Proof         pldtypes.HexBytes `json:"proof"`
	Options       pldtypes.HexBytes `json:"options"`
}

type NotoSetLockOptionsParams struct {
	TxId         string            `json:"txId"`
	LockID       pldtypes.Bytes32  `json:"lockId"`
	LockedInputs []string          `json:"lockedInputs"`
	Options      pldtypes.HexBytes `json:"options"`
	Proof        pldtypes.HexBytes `json:"proof"`
	Data         pldtypes.HexBytes `json:"data"`
}

type UpdateLockParams struct {
	LockID pldtypes.Bytes32     `json:"lockId"`
	Params NotoUpdateLockParams `json:"params"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type NotoUpdateLockParams struct {
	TxId         string            `json:"txId"`
	LockedInputs []string          `json:"lockedInputs"`
	Proof        pldtypes.HexBytes `json:"proof"`
	Options      pldtypes.HexBytes `json:"options"`
}

var UpdateLockParamsABI = &abi.ParameterArray{
	{Name: "txId", Type: "bytes32"},
	{Name: "lockedInputs", Type: "bytes32[]"},
	{Name: "proof", Type: "bytes"},
	{Name: "options", Type: "bytes"},
}

type NotoDelegateLockParams struct {
	LockID     pldtypes.Bytes32     `json:"lockId"`
	NewSpender *pldtypes.EthAddress `json:"newSpender"`
	Data       pldtypes.HexBytes    `json:"data"`
}

type DelegateLockData struct {
	TxId pldtypes.Bytes32  `json:"txId"`
	Data pldtypes.HexBytes `json:"data"`
}

type DelegateLockDataStrings struct {
	TxId string            `json:"txId"`
	Data pldtypes.HexBytes `json:"data"`
}

var DelegateLockDataABI = &abi.ParameterArray{
	{Name: "txId", Type: "bytes32"},
	{Name: "data", Type: "bytes"},
}

type UnlockData struct {
	TxId    pldtypes.Bytes32   `json:"txId"`
	Inputs  []pldtypes.Bytes32 `json:"inputs"`
	Outputs []pldtypes.Bytes32 `json:"outputs"`
	Data    pldtypes.HexBytes  `json:"data"`
}

type UnlockDataStrings struct {
	TxId    string            `json:"txId"`
	Inputs  []string          `json:"inputs"`
	Outputs []string          `json:"outputs"`
	Data    pldtypes.HexBytes `json:"data"`
}

var UnlockDataABI = &abi.ParameterArray{
	{Name: "txId", Type: "bytes32"},
	{Name: "inputs", Type: "bytes32[]"},
	{Name: "outputs", Type: "bytes32[]"},
	{Name: "data", Type: "bytes"},
}

type NotoTransfer_Event struct {
	TxId     pldtypes.Bytes32     `json:"txId"`
	Operator *pldtypes.EthAddress `json:"operator"`
	Inputs   []pldtypes.Bytes32   `json:"inputs"`
	Outputs  []pldtypes.Bytes32   `json:"outputs"`
	Proof    pldtypes.HexBytes    `json:"proof"`
	Data     pldtypes.HexBytes    `json:"data"`
}

type LockStates struct {
	Inputs        []pldtypes.Bytes32 `json:"inputs"`
	Outputs       []pldtypes.Bytes32 `json:"outputs"`
	LockedOutputs []pldtypes.Bytes32 `json:"lockedOutputs"`
}

type NotoLockCreated_Event struct {
	TxId          pldtypes.Bytes32     `json:"txId"`
	LockID        pldtypes.Bytes32     `json:"lockId"`
	Owner         *pldtypes.EthAddress `json:"owner"`
	Spender       *pldtypes.EthAddress `json:"spender"`
	Inputs        []pldtypes.Bytes32   `json:"inputs"`
	Outputs       []pldtypes.Bytes32   `json:"outputs"`
	LockedOutputs []pldtypes.Bytes32   `json:"lockedOutputs"`
	Proof         pldtypes.HexBytes    `json:"proof"`
	Data          pldtypes.HexBytes    `json:"data"`
}

type NotoLockSpent_Event struct {
	LockID  pldtypes.Bytes32     `json:"lockId"`
	Spender *pldtypes.EthAddress `json:"spender"`
	Data    pldtypes.HexBytes    `json:"data"`
}

type NotoLockCancelled_Event struct {
	LockID  pldtypes.Bytes32     `json:"lockId"`
	Spender *pldtypes.EthAddress `json:"spender"`
	Data    pldtypes.HexBytes    `json:"data"`
}

type NotoLockUpdated_Event struct {
	TxId         pldtypes.Bytes32     `json:"txId"`
	LockID       pldtypes.Bytes32     `json:"lockId"`
	Operator     *pldtypes.EthAddress `json:"operator"`
	LockedInputs []pldtypes.Bytes32   `json:"lockedInputs"`
	Proof        pldtypes.HexBytes    `json:"proof"`
	Data         pldtypes.HexBytes    `json:"data"`
}

type NotoLockDelegated_Event struct {
	LockID pldtypes.Bytes32     `json:"lockId"`
	From   *pldtypes.EthAddress `json:"from"`
	To     *pldtypes.EthAddress `json:"to"`
	Data   pldtypes.HexBytes    `json:"data"`
}

type parsedCoins struct {
	coins        []*types.NotoCoin
	states       []*prototk.StateRef
	total        *big.Int
	lockedCoins  []*types.NotoLockedCoin
	lockedStates []*prototk.StateRef
	lockedTotal  *big.Int
}

// Variant 0 parameter structures (legacy)
type NotoTransfer_V0_Params struct {
	TxId      string            `json:"txId"`
	Inputs    []string          `json:"inputs"`
	Outputs   []string          `json:"outputs"`
	Signature pldtypes.HexBytes `json:"signature"`
	Data      pldtypes.HexBytes `json:"data"`
}

type NotoMint_V0_Params struct {
	TxId      string            `json:"txId"`
	Outputs   []string          `json:"outputs"`
	Signature pldtypes.HexBytes `json:"signature"`
	Data      pldtypes.HexBytes `json:"data"`
}

type NotoLock_V0_Params struct {
	TxId          string            `json:"txId"`
	Inputs        []string          `json:"inputs"`
	Outputs       []string          `json:"outputs"`
	LockedOutputs []string          `json:"lockedOutputs"`
	Signature     pldtypes.HexBytes `json:"signature"`
	Data          pldtypes.HexBytes `json:"data"`
}

type NotoUnlock_V0_Params struct {
	TxId          string            `json:"txId"`
	LockedInputs  []string          `json:"lockedInputs"`
	LockedOutputs []string          `json:"lockedOutputs"`
	Outputs       []string          `json:"outputs"`
	Signature     pldtypes.HexBytes `json:"signature"`
	Data          pldtypes.HexBytes `json:"data"`
}

type NotoPrepareUnlock_V0_Params struct {
	LockedInputs []string          `json:"lockedInputs"`
	UnlockHash   string            `json:"unlockHash"`
	Signature    pldtypes.HexBytes `json:"signature"`
	Data         pldtypes.HexBytes `json:"data"`
}

type NotoDelegateLock_V0_Params struct {
	TxId       string               `json:"txId"`
	UnlockHash string               `json:"unlockHash"`
	Delegate   *pldtypes.EthAddress `json:"delegate"`
	Signature  pldtypes.HexBytes    `json:"signature"`
	Data       pldtypes.HexBytes    `json:"data"`
}

// Old event structures for variant 0 compatibility
type NotoTransfer_V0_Event struct {
	TxId      pldtypes.Bytes32   `json:"txId"`
	Inputs    []pldtypes.Bytes32 `json:"inputs"`
	Outputs   []pldtypes.Bytes32 `json:"outputs"`
	Signature pldtypes.HexBytes  `json:"signature"`
	Data      pldtypes.HexBytes  `json:"data"`
}

type NotoLock_V0_Event struct {
	TxId          pldtypes.Bytes32   `json:"txId"`
	Inputs        []pldtypes.Bytes32 `json:"inputs"`
	Outputs       []pldtypes.Bytes32 `json:"outputs"`
	LockedOutputs []pldtypes.Bytes32 `json:"lockedOutputs"`
	Signature     pldtypes.HexBytes  `json:"signature"`
	Data          pldtypes.HexBytes  `json:"data"`
}

type NotoUnlock_V0_Event struct {
	TxId          pldtypes.Bytes32    `json:"txId"`
	Sender        pldtypes.EthAddress `json:"sender"`
	LockedInputs  []pldtypes.Bytes32  `json:"lockedInputs"`
	LockedOutputs []pldtypes.Bytes32  `json:"lockedOutputs"`
	Outputs       []pldtypes.Bytes32  `json:"outputs"`
	Signature     pldtypes.HexBytes   `json:"signature"`
	Data          pldtypes.HexBytes   `json:"data"`
}

type NotoUnlockPrepared_V0_Event struct {
	LockedInputs []pldtypes.Bytes32 `json:"lockedInputs"`
	UnlockHash   pldtypes.Bytes32   `json:"unlockHash"`
	Signature    pldtypes.HexBytes  `json:"signature"`
	Data         pldtypes.HexBytes  `json:"data"`
}

type NotoLockDelegated_V0_Event struct {
	TxId       pldtypes.Bytes32    `json:"txId"`
	UnlockHash pldtypes.Bytes32    `json:"unlockHash"`
	Delegate   pldtypes.EthAddress `json:"delegate"`
	Signature  pldtypes.HexBytes   `json:"signature"`
	Data       pldtypes.HexBytes   `json:"data"`
}

func mustLoadEventSignatures(contractABI abi.ABI, allEvents []string) map[string]string {
	events := contractABI.Events()
	signatures := make(map[string]string, len(allEvents))
	for _, eventName := range allEvents {
		event := events[eventName]
		if event == nil {
			panic(fmt.Errorf("unknown event: %s", eventName))
		}
		signatures[eventName] = event.SolString()
	}
	return signatures
}

func mustParseJSON[T any](obj T) string {
	parsed, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(parsed)
}

func mustBuildEventsJSON(abis ...abi.ABI) string {
	var events abi.ABI
	for _, a := range abis {
		for _, entry := range a {
			// We include errors as well as events, so that Paladin will decode domain errors
			if entry.Type == abi.Event || entry.Type == abi.Error {
				events = append(events, entry)
			}
		}
	}
	return mustParseJSON(events)
}

func mustParseSchemas(allSchemas []*abi.Parameter) []string {
	schemas := make([]string, len(allSchemas))
	for i, schema := range allSchemas {
		schemas[i] = mustParseJSON(schema)
	}
	return schemas
}

func (n *Noto) Name() string {
	return n.name
}

func (n *Noto) CoinSchemaID() string {
	return n.coinSchema.Id
}

func (n *Noto) LockedCoinSchemaID() string {
	return n.lockedCoinSchema.Id
}

func (n *Noto) LockInfoSchemaID() string {
	return n.lockInfoSchema.Id
}

func (n *Noto) DataSchemaID() string {
	return n.dataSchema.Id
}

func (n *Noto) ConfigureDomain(ctx context.Context, req *prototk.ConfigureDomainRequest) (*prototk.ConfigureDomainResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	var config types.DomainConfig
	err := json.Unmarshal([]byte(req.ConfigJson), &config)
	if err != nil {
		return nil, err
	}

	n.name = req.Name
	n.config = config
	n.chainID = req.ChainId
	n.fixedSigningIdentity = req.FixedSigningIdentity

	return &prototk.ConfigureDomainResponse{
		DomainConfig: &prototk.DomainConfig{
			AbiStateSchemasJson: schemasJSON,
			AbiEventsJson:       allEventsJSON,
		},
	}, nil
}

func (n *Noto) InitDomain(ctx context.Context, req *prototk.InitDomainRequest) (*prototk.InitDomainResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	for i, schema := range allSchemas {
		switch schema.Name {
		case types.NotoCoinABI.Name:
			n.coinSchema = req.AbiStateSchemas[i]
		case types.NotoLockedCoinABI.Name:
			n.lockedCoinSchema = req.AbiStateSchemas[i]
		case types.TransactionDataABI.Name:
			n.dataSchema = req.AbiStateSchemas[i]
		case types.NotoLockInfoABI.Name:
			n.lockInfoSchema = req.AbiStateSchemas[i]
		}
	}
	return &prototk.InitDomainResponse{}, nil
}

func (n *Noto) InitDeploy(ctx context.Context, req *prototk.InitDeployRequest) (*prototk.InitDeployResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	params, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}

	switch params.NotaryMode {
	case types.NotaryModeBasic:
		// no required params
	case types.NotaryModeHooks:
		if params.Options.Hooks == nil {
			return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "options.hooks")
		}
		if params.Options.Hooks.PublicAddress == nil {
			return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "options.hooks.publicAddress")
		}
		if !params.Options.Hooks.DevUsePublicHooks {
			if params.Options.Hooks.PrivateAddress == nil {
				return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "options.hooks.privateAddress")
			}
			if params.Options.Hooks.PrivateGroup == nil {
				return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "options.hooks.privateGroup")
			}
		}
	default:
		return nil, i18n.NewError(ctx, msgs.MsgParameterRequired, "notaryMode")
	}

	return &prototk.InitDeployResponse{
		RequiredVerifiers: []*prototk.ResolveVerifierRequest{
			{
				Lookup:       params.Notary,
				Algorithm:    algorithms.ECDSA_SECP256K1,
				VerifierType: verifiers.ETH_ADDRESS,
			},
		},
	}, nil
}

func (n *Noto) PrepareDeploy(ctx context.Context, req *prototk.PrepareDeployRequest) (*prototk.PrepareDeployResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	params, err := n.validateDeploy(req.Transaction)
	if err != nil {
		return nil, err
	}
	localNodeName, _ := n.Callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	notaryQualified, err := pldtypes.PrivateIdentityLocator(params.Notary).FullyQualified(ctx, localNodeName.Name)
	if err != nil {
		return nil, err
	}
	notaryAddress, err := n.findEthAddressVerifier(ctx, "notary", params.Notary, req.ResolvedVerifiers)
	if err != nil {
		return nil, err
	}

	deployData := &types.NotoConfigData_V0{
		NotaryLookup: notaryQualified.String(),
	}
	switch params.NotaryMode {
	case types.NotaryModeBasic:
		deployData.NotaryMode = types.NotaryModeIntBasic
		deployData.RestrictMint = true
		deployData.AllowBurn = true
		deployData.AllowLock = true
		if params.Options.Basic != nil {
			if params.Options.Basic.RestrictMint != nil {
				deployData.RestrictMint = *params.Options.Basic.RestrictMint
			}
			if params.Options.Basic.AllowBurn != nil {
				deployData.AllowBurn = *params.Options.Basic.AllowBurn
			}
			if params.Options.Basic.AllowLock != nil {
				deployData.AllowLock = *params.Options.Basic.AllowLock
			}
		}
	case types.NotaryModeHooks:
		deployData.NotaryMode = types.NotaryModeIntHooks
		deployData.PrivateAddress = params.Options.Hooks.PrivateAddress
		deployData.PrivateGroup = params.Options.Hooks.PrivateGroup
		notaryAddress = params.Options.Hooks.PublicAddress
	}

	var functionJSON []byte
	var paramsJSON []byte
	var deployDataJSON []byte

	signer := n.fixedSigningIdentity
	if signer == "" {
		// Use a random key to deploy if no default signing identity is set
		signer = fmt.Sprintf("%s.deploy.%s", n.name, uuid.New())
	}

	// Default to the V0 NotoFactory ABI if no version is specified
	abi := factoryV0Build.ABI
	if n.config.FactoryVersion == 1 {
		abi = factoryBuild.ABI
	}

	functionName := "deploy"
	if params.Implementation != "" {
		functionName = "deployImplementation"
	}
	functionJSON, err = json.Marshal(abi.Functions()[functionName])
	if err == nil {
		deployDataJSON, err = json.Marshal(deployData)
	}
	if err == nil {
		// For V0 factories, we need to omit name and symbol parameters
		if n.config.FactoryVersion == 0 {
			paramsJSON, err = json.Marshal(&NotoDeployParams{
				TransactionID: req.Transaction.TransactionId,
				Notary:        *notaryAddress,
				Data:          deployDataJSON,
			})
		} else {
			// For V1 factories, include name and symbol
			paramsJSON, err = json.Marshal(&NotoDeployParams{
				TransactionID: req.Transaction.TransactionId,
				Name:          params.Name,
				Symbol:        params.Symbol,
				Notary:        *notaryAddress,
				Data:          deployDataJSON,
			})
		}
	}

	return &prototk.PrepareDeployResponse{
		Transaction: &prototk.PreparedTransaction{
			FunctionAbiJson: string(functionJSON),
			ParamsJson:      string(paramsJSON),
		},
		Signer: &signer,
	}, err
}

func (n *Noto) InitContract(ctx context.Context, req *prototk.InitContractRequest) (*prototk.InitContractResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	var notoContractConfigJSON []byte

	domainConfig, decodedData, err := n.decodeConfig(ctx, req.ContractConfig)
	if err != nil {
		// This on-chain contract has invalid configuration - not an error in our process
		log.L(ctx).Errorf("Error decoding config: %s", err)
		return &prototk.InitContractResponse{Valid: false}, nil
	}

	localNodeName, _ := n.Callbacks.LocalNodeName(ctx, &prototk.LocalNodeNameRequest{})
	_, notaryNodeName, err := pldtypes.PrivateIdentityLocator(decodedData.NotaryLookup).Validate(ctx, localNodeName.Name, true)
	if err != nil {
		return nil, err
	}

	parsedConfig := &types.NotoParsedConfig{
		Name:         domainConfig.Name,
		Symbol:       domainConfig.Symbol,
		Decimals:     domainConfig.Decimals,
		NotaryMode:   types.NotaryModeBasic.Enum(),
		Variant:      domainConfig.Variant,
		NotaryLookup: decodedData.NotaryLookup,
		IsNotary:     notaryNodeName == localNodeName.Name,
	}
	if decodedData.NotaryMode == types.NotaryModeIntHooks {
		parsedConfig.NotaryMode = types.NotaryModeHooks.Enum()
		parsedConfig.Options.Hooks = &types.NotoHooksOptions{
			PublicAddress:     &domainConfig.NotaryAddress,
			PrivateAddress:    decodedData.PrivateAddress,
			PrivateGroup:      decodedData.PrivateGroup,
			DevUsePublicHooks: decodedData.PrivateAddress == nil,
		}
	} else {
		parsedConfig.Options.Basic = &types.NotoBasicOptions{
			RestrictMint: &decodedData.RestrictMint,
			AllowBurn:    &decodedData.AllowBurn,
			AllowLock:    &decodedData.AllowLock,
		}
	}

	notoContractConfigJSON, err = json.Marshal(parsedConfig)
	return &prototk.InitContractResponse{
		Valid: true,
		ContractConfig: &prototk.ContractConfig{
			ContractConfigJson:   string(notoContractConfigJSON),
			CoordinatorSelection: prototk.ContractConfig_COORDINATOR_STATIC,
			StaticCoordinator:    &decodedData.NotaryLookup,
			SubmitterSelection:   prototk.ContractConfig_SUBMITTER_COORDINATOR,
		},
	}, err
}

func (n *Noto) InitTransaction(ctx context.Context, req *prototk.InitTransactionRequest) (*prototk.InitTransactionResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Init(ctx, tx, req)
}

func (n *Noto) AssembleTransaction(ctx context.Context, req *prototk.AssembleTransactionRequest) (*prototk.AssembleTransactionResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Assemble(ctx, tx, req)
}

func (n *Noto) EndorseTransaction(ctx context.Context, req *prototk.EndorseTransactionRequest) (*prototk.EndorseTransactionResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Endorse(ctx, tx, req)
}

func (n *Noto) PrepareTransaction(ctx context.Context, req *prototk.PrepareTransactionRequest) (*prototk.PrepareTransactionResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	tx, handler, err := n.validateTransaction(ctx, req.Transaction)
	if err != nil {
		return nil, err
	}
	return handler.Prepare(ctx, tx, req)
}

func (n *Noto) decodeConfig(ctx context.Context, domainConfig []byte) (*types.NotoConfig_V1, *types.NotoConfigData_V0, error) {
	var configSelector ethtypes.HexBytes0xPrefix
	if len(domainConfig) >= 4 {
		configSelector = ethtypes.HexBytes0xPrefix(domainConfig[0:4])
	}

	var err error
	var configValues *abi.ComponentValue
	switch configSelector.String() {
	case types.NotoConfigID_V0.String():
		configValues, err = types.NotoConfigABI_V0.DecodeABIDataCtx(ctx, domainConfig[4:], 0)
		if err != nil {
			return nil, nil, err
		}
	case types.NotoConfigID_V1.String():
		configValues, err = types.NotoConfigABI_V1.DecodeABIDataCtx(ctx, domainConfig[4:], 0)
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, i18n.NewError(ctx, msgs.MsgUnexpectedConfigType, configSelector)
	}

	var config types.NotoConfig_V1
	var decodedData types.NotoConfigData_V0
	configJSON, err := pldtypes.StandardABISerializer().SerializeJSON(configValues)
	if err == nil {
		err = json.Unmarshal(configJSON, &config)
	}
	if err == nil {
		err = json.Unmarshal(config.Data, &decodedData)
	}
	return &config, &decodedData, err
}

func (n *Noto) validateDeploy(tx *prototk.DeployTransactionSpecification) (*types.ConstructorParams, error) {
	var params types.ConstructorParams
	err := json.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	if err == nil && params.Notary == "" {
		err = i18n.NewError(context.Background(), msgs.MsgParameterRequired, "notary")
	}
	return &params, err
}

func validateTransactionCommon[T any](
	ctx context.Context,
	tx *prototk.TransactionSpecification,
	getHandler func(method string) T,
) (*types.ParsedTransaction, T, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		var zero T
		return nil, zero, err
	}

	var domainConfig types.NotoParsedConfig
	err = json.Unmarshal([]byte(tx.ContractInfo.ContractConfigJson), &domainConfig)
	if err != nil {
		var zero T
		return nil, zero, err
	}

	abi := types.NotoABI.Functions()[functionABI.Name]
	handler := getHandler(functionABI.Name)
	handlerValue := reflect.ValueOf(handler)
	if abi == nil || handlerValue.IsNil() {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgUnknownFunction, functionABI.Name)
	}

	// check if the handler implements the ValidateParams method cause generic T
	validator, ok := any(handler).(ParamValidator)
	if !ok {
		var zero T
		return nil, zero, i18n.NewError(ctx, msgs.MsgErrorHandlerImplementationNotFound)
	}

	params, err := validator.ValidateParams(ctx, &domainConfig, tx.FunctionParamsJson)
	if err != nil {
		var zero T
		return nil, zero, err
	}

	signature, err := abi.SolidityStringCtx(ctx)
	if err == nil && tx.FunctionSignature != signature {
		err = i18n.NewError(ctx, msgs.MsgUnexpectedFunctionSignature, functionABI.Name, signature, tx.FunctionSignature)
	}
	if err != nil {
		var zero T
		return nil, zero, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractInfo.ContractAddress)
	if err != nil {
		var zero T
		return nil, zero, err
	}

	return &types.ParsedTransaction{
		Transaction:     tx,
		FunctionABI:     &functionABI,
		ContractAddress: contractAddress,
		DomainConfig:    &domainConfig,
		Params:          params,
	}, handler, nil
}

func (n *Noto) validateTransaction(ctx context.Context, tx *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainHandler, error) {
	return validateTransactionCommon(
		ctx,
		tx,
		n.GetHandler,
	)
}

func (n *Noto) validateCall(ctx context.Context, call *prototk.TransactionSpecification) (*types.ParsedTransaction, types.DomainCallHandler, error) {
	return validateTransactionCommon(
		ctx,
		call,
		n.GetCallHandler,
	)
}

func (n *Noto) ethAddressVerifiers(lookups ...string) []*prototk.ResolveVerifierRequest {
	verifierMap := make(map[string]bool, len(lookups))
	verifierList := make([]string, 0, len(lookups))
	for _, lookup := range lookups {
		if _, ok := verifierMap[lookup]; !ok {
			verifierMap[lookup] = true
			verifierList = append(verifierList, lookup)
		}
	}
	request := make([]*prototk.ResolveVerifierRequest, len(verifierList))
	for i, lookup := range verifierList {
		request[i] = &prototk.ResolveVerifierRequest{
			Lookup:       lookup,
			Algorithm:    algorithms.ECDSA_SECP256K1,
			VerifierType: verifiers.ETH_ADDRESS,
		}
	}
	return request
}

func (n *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}

func (n *Noto) parseCoinList(ctx context.Context, label string, states []*prototk.EndorsableState) (*parsedCoins, error) {
	statesUsed := make(map[string]bool)
	result := &parsedCoins{
		total:       new(big.Int),
		lockedTotal: new(big.Int),
	}
	for i, state := range states {
		if statesUsed[state.Id] {
			return nil, i18n.NewError(ctx, msgs.MsgDuplicateStateInList, label, i, state.Id)
		}
		statesUsed[state.Id] = true

		switch state.SchemaId {
		case n.coinSchema.Id:
			coin, err := n.unmarshalCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			result.coins = append(result.coins, coin)
			result.total = result.total.Add(result.total, coin.Amount.Int())
			result.states = append(result.states, &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})

		case n.lockedCoinSchema.Id:
			coin, err := n.unmarshalLockedCoin(state.StateDataJson)
			if err != nil {
				return nil, i18n.NewError(ctx, msgs.MsgInvalidListInput, label, i, state.Id, err)
			}
			result.lockedCoins = append(result.lockedCoins, coin)
			result.lockedTotal = result.lockedTotal.Add(result.lockedTotal, coin.Amount.Int())
			result.lockedStates = append(result.lockedStates, &prototk.StateRef{
				SchemaId: state.SchemaId,
				Id:       state.Id,
			})

		default:
			return nil, i18n.NewError(ctx, msgs.MsgUnexpectedSchema, state.SchemaId)
		}
	}
	return result, nil
}

func (n *Noto) encodeTransactionData_V0(ctx context.Context, transactionID string, infoStates []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
	var err error
	stateIDs := make([]pldtypes.Bytes32, len(infoStates))
	for i, state := range infoStates {
		stateIDs[i], err = pldtypes.ParseBytes32Ctx(ctx, state.Id)
		if err != nil {
			return nil, err
		}
	}

	dataValues := &types.NotoTransactionData_V0{
		TransactionID: pldtypes.MustParseBytes32(transactionID),
		InfoStates:    stateIDs,
	}
	dataJSON, err := json.Marshal(dataValues)
	if err != nil {
		return nil, err
	}
	dataABI, err := types.NotoTransactionDataABI_V0.EncodeABIDataJSONCtx(ctx, dataJSON)
	if err != nil {
		return nil, err
	}

	var data []byte
	data = append(data, types.NotoTransactionDataID_V0...)
	data = append(data, dataABI...)
	return data, nil
}

func (n *Noto) encodeTransactionData(ctx context.Context, infoStates []*prototk.EndorsableState) (pldtypes.HexBytes, error) {
	var err error
	stateIDs := make([]pldtypes.Bytes32, len(infoStates))
	for i, state := range infoStates {
		stateIDs[i], err = pldtypes.ParseBytes32Ctx(ctx, state.Id)
		if err != nil {
			return nil, err
		}
	}

	dataValues := &types.NotoTransactionData_V1{
		InfoStates: stateIDs,
	}
	dataJSON, err := json.Marshal(dataValues)
	if err != nil {
		return nil, err
	}
	dataABI, err := types.NotoTransactionDataABI_V1.EncodeABIDataJSONCtx(ctx, dataJSON)
	if err != nil {
		return nil, err
	}

	var data []byte
	data = append(data, types.NotoTransactionDataID_V1...)
	data = append(data, dataABI...)
	return data, nil
}

func (n *Noto) decodeTransactionData(ctx context.Context, data pldtypes.HexBytes) (*types.NotoTransactionData_V0, error) {
	var dataValues types.NotoTransactionData_V0
	if len(data) >= 4 {
		dataPrefix := data[0:4]

		switch dataPrefix.String() {
		case types.NotoTransactionDataID_V1.String():
			dataDecoded, err := types.NotoTransactionDataABI_V1.DecodeABIDataCtx(ctx, data, 4)
			if err == nil {
				var dataJSON []byte
				dataJSON, err = dataDecoded.JSON()
				if err == nil {
					// Note that V1 is a subset of V0, so we can use the same struct
					err = json.Unmarshal(dataJSON, &dataValues)
					if err == nil {
						return &dataValues, nil
					}
				}
			}
			if err != nil {
				return nil, err
			}

		case types.NotoTransactionDataID_V0.String():
			dataDecoded, err := types.NotoTransactionDataABI_V0.DecodeABIDataCtx(ctx, data, 4)
			if err == nil {
				var dataJSON []byte
				dataJSON, err = dataDecoded.JSON()
				if err == nil {
					err = json.Unmarshal(dataJSON, &dataValues)
					if err == nil {
						return &dataValues, nil
					}
				}
			}
			if err != nil {
				return nil, err
			}
		}
	}

	return &dataValues, nil
}

func (n *Noto) wrapHookTransaction(domainConfig *types.NotoParsedConfig, functionABI *abi.Entry, params any) (pldapi.TransactionType, *abi.Entry, pldtypes.HexBytes, error) {
	if domainConfig.Options.Hooks.DevUsePublicHooks {
		paramsJSON, err := json.Marshal(params)
		return pldapi.TransactionTypePublic, functionABI, paramsJSON, err
	}

	functionABI = penteInvokeABI(functionABI.Name, functionABI.Inputs)
	penteParams := &PenteInvokeParams{
		Group:  domainConfig.Options.Hooks.PrivateGroup,
		To:     domainConfig.Options.Hooks.PrivateAddress,
		Inputs: params,
	}
	paramsJSON, err := json.Marshal(penteParams)
	return pldapi.TransactionTypePrivate, functionABI, paramsJSON, err
}

func mapSendTransactionType(transactionType pldapi.TransactionType) prototk.TransactionInput_TransactionType {
	if transactionType == pldapi.TransactionTypePrivate {
		return prototk.TransactionInput_PRIVATE
	}
	return prototk.TransactionInput_PUBLIC
}

func mapPrepareTransactionType(transactionType pldapi.TransactionType) prototk.PreparedTransaction_TransactionType {
	if transactionType == pldapi.TransactionTypePrivate {
		return prototk.PreparedTransaction_PRIVATE
	}
	return prototk.PreparedTransaction_PUBLIC
}

func (n *Noto) Sign(ctx context.Context, req *prototk.SignRequest) (*prototk.SignResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) GetVerifier(ctx context.Context, req *prototk.GetVerifierRequest) (*prototk.GetVerifierResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) ValidateStateHashes(ctx context.Context, req *prototk.ValidateStateHashesRequest) (*prototk.ValidateStateHashesResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) InitCall(ctx context.Context, req *prototk.InitCallRequest) (*prototk.InitCallResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	ptx, handler, err := n.validateCall(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateInitCallTxSpec, err)
	}
	return handler.InitCall(ctx, ptx, req)
}

func (n *Noto) ExecCall(ctx context.Context, req *prototk.ExecCallRequest) (*prototk.ExecCallResponse, error) {
	ctx = log.WithComponent(ctx, "noto")
	ptx, handler, err := n.validateCall(ctx, req.Transaction)
	if err != nil {
		return nil, i18n.NewError(ctx, msgs.MsgErrorValidateExecCallTxSpec, err)
	}
	return handler.ExecCall(ctx, ptx, req)
}

func (n *Noto) ConfigurePrivacyGroup(ctx context.Context, req *prototk.ConfigurePrivacyGroupRequest) (*prototk.ConfigurePrivacyGroupResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) InitPrivacyGroup(ctx context.Context, req *prototk.InitPrivacyGroupRequest) (*prototk.InitPrivacyGroupResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

func (n *Noto) WrapPrivacyGroupEVMTX(ctx context.Context, req *prototk.WrapPrivacyGroupEVMTXRequest) (*prototk.WrapPrivacyGroupEVMTXResponse, error) {
	return nil, i18n.NewError(ctx, msgs.MsgNotImplemented)
}

// getInterfaceABI returns the appropriate interface ABI based on the variant
func (n *Noto) getInterfaceABI(variant pldtypes.HexUint64) abi.ABI {
	if variant == types.NotoVariantLegacy {
		return interfaceV0Build.ABI
	}
	return interfaceBuild.ABI
}

// computeLockId computes the lockId the same way the contract does:
// keccak256(abi.encode(address(this), msg.sender, txId))
func (n *Noto) computeLockId(ctx context.Context, contractAddress *pldtypes.EthAddress, notaryAddress *pldtypes.EthAddress, txId string) (pldtypes.Bytes32, error) {
	params := abi.ParameterArray{
		{Name: "contract", Type: "address"},
		{Name: "notary", Type: "address"},
		{Name: "txId", Type: "bytes32"},
	}

	paramsJSON := map[string]any{
		"contract": contractAddress.String(),
		"notary":   notaryAddress.String(),
		"txId":     txId,
	}

	jsonData, err := json.Marshal(paramsJSON)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}

	encoded, err := params.EncodeABIDataJSONCtx(ctx, jsonData)
	if err != nil {
		return pldtypes.Bytes32{}, err
	}

	return pldtypes.Bytes32Keccak(encoded), nil
}
