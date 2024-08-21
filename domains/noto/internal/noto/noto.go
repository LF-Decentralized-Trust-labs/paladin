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
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/signer/api"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v2"
)

//go:embed abis/NotoFactory.json
var notoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/Noto.json
var notoJSON []byte // From "gradle copySolidity"

var (
	fromDomain = "from-domain"
)

type Config struct {
	FactoryAddress string `json:"factoryAddress" yaml:"factoryAddress"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type Noto struct {
	Factory      SolidityBuild
	Contract     SolidityBuild
	conn         *grpc.ClientConn
	dest         *string
	client       pb.KataMessageServiceClient
	stream       pb.KataMessageService_ListenClient
	stopListener context.CancelFunc
	done         chan bool
	chainID      int64
	domainID     string
	coinSchema   *pb.StateSchema
	replies      *replyTracker
}

type NotoDomainConfig struct {
	Notary string `json:"notary"`
}

var NotoDomainConfigABI = &abi.ParameterArray{
	{Name: "notary", Type: "address"},
}

type parsedTransaction struct {
	contractAddress *ethtypes.Address0xHex
	domainConfig    *NotoDomainConfig
	params          interface{}
}

type assemblyResult struct {
	spentCoins  []*NotoCoin
	spentStates []*pb.StateRef
	newCoins    []*NotoCoin
	newStates   []*pb.NewState
}

func loadBuild(buildOutput []byte) SolidityBuild {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	return build
}

func findMethod(entries []*abi.Entry, name string) *abi.Entry {
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func New(ctx context.Context, addr string) (*Noto, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}

	contract := loadBuild(notoJSON)
	transfer := findMethod(contract.ABI, "transfer")
	if transfer != nil {
		// Add names for unused parameters in this contract variant
		// (underlying library does not allow unnamed parameters)
		transfer.Inputs[2].Name = "signature"
	}

	d := &Noto{
		conn:     conn,
		client:   pb.NewKataMessageServiceClient(conn),
		Factory:  loadBuild(notoFactoryJSON),
		Contract: contract,
	}
	d.replies = &replyTracker{
		inflight: make(map[string]*inflightRequest),
		client:   d.client,
	}
	return d, d.waitForReady(ctx)
}

func (d *Noto) waitForReady(ctx context.Context) error {
	status, err := d.client.Status(ctx, &pb.StatusRequest{})
	delay := 0
	for !status.GetOk() {
		time.Sleep(time.Second)
		delay++
		if delay > 2 {
			return fmt.Errorf("server was not ready after 2 seconds")
		}
		status, err = d.client.Status(ctx, &pb.StatusRequest{})
	}
	if err != nil {
		return err
	}
	if !status.GetOk() {
		return fmt.Errorf("got non-OK status from server")
	}
	return nil
}

func (d *Noto) Close() error {
	if d.stream != nil {
		if err := d.stream.CloseSend(); err != nil {
			return err
		}
		d.done <- true
		d.stopListener()
	}
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Noto) Listen(ctx context.Context, dest string) error {
	d.dest = &dest
	d.done = make(chan bool, 1)

	var err error
	var listenerContext context.Context

	listenerContext, d.stopListener = context.WithCancel(ctx)
	d.stream, err = d.client.Listen(listenerContext, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return fmt.Errorf("failed to listen for domain events: %v", err)
	}

	handlerCtx := log.WithLogField(ctx, "role", "handler")
	go d.handler(handlerCtx)
	return nil
}

func (d *Noto) sendReply(ctx context.Context, message *pb.Message, reply proto.Message) error {
	body, err := anypb.New(reply)
	if err == nil {
		_, err = d.client.SendMessage(ctx, &pb.Message{
			Destination:   *message.ReplyTo,
			CorrelationId: &message.Id,
			Body:          body,
			ReplyTo:       d.dest,
		})
	}
	return err
}

func (d *Noto) handler(ctx context.Context) {
	for {
		in, err := d.stream.Recv()
		select {
		case <-d.done:
			return
		default:
			// do nothing
		}
		if err != nil {
			log.L(ctx).Errorf("Error receiving message - terminating handler loop: %v", err)
			return
		}

		// TODO: should probably have a finite number of workers?
		// Cannot be synchronous, as some calls ("assemble") may need to make
		// their own additional calls ("find states") and receive the results
		go func() {
			reply, err := d.handleMessage(ctx, in)
			if err != nil {
				reply = &pb.DomainAPIError{ErrorMessage: err.Error()}
				err = nil
			}
			if reply != nil {
				if err = d.sendReply(ctx, in, reply); err != nil {
					log.L(ctx).Errorf("Error sending message reply: %s", err)
				}
			}
		}()
	}
}

func (d *Noto) handleMessage(ctx context.Context, message *pb.Message) (reply proto.Message, err error) {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return nil, err
	}

	inflight := d.replies.getInflight(message.CorrelationId)
	if inflight != nil {
		inflight.done <- message
		return nil, nil
	}

	switch m := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.L(ctx).Infof("Received ConfigureDomainRequest")
		d.chainID = m.ChainId

		var config Config
		err := yaml.Unmarshal([]byte(m.ConfigYaml), &config)
		if err != nil {
			return nil, err
		}
		factoryJSON, err := json.Marshal(d.Factory.ABI)
		if err != nil {
			return nil, err
		}
		notoJSON, err := json.Marshal(d.Contract.ABI)
		if err != nil {
			return nil, err
		}
		constructorJSON, err := json.Marshal(NotoConstructorABI)
		if err != nil {
			return nil, err
		}
		schemaJSON, err := json.Marshal(NotoCoinABI)
		if err != nil {
			return nil, err
		}

		return &pb.ConfigureDomainResponse{
			DomainConfig: &pb.DomainConfig{
				FactoryContractAddress: config.FactoryAddress,
				FactoryContractAbiJson: string(factoryJSON),
				PrivateContractAbiJson: string(notoJSON),
				ConstructorAbiJson:     string(constructorJSON),
				AbiStateSchemasJson:    []string{string(schemaJSON)},
			},
		}, nil

	case *pb.InitDomainRequest:
		log.L(ctx).Infof("Received InitDomainRequest")
		d.domainID = m.DomainUuid
		d.coinSchema = m.AbiStateSchemas[0]
		return &pb.InitDomainResponse{}, nil

	case *pb.InitDeployTransactionRequest:
		log.L(ctx).Infof("Received InitDeployTransactionRequest")

		params, err := d.validateDeploy(m.Transaction)
		if err != nil {
			return nil, err
		}

		return &pb.InitDeployTransactionResponse{
			RequiredVerifiers: []*pb.ResolveVerifierRequest{
				{
					Lookup:    params.Notary,
					Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				},
			},
		}, nil

	case *pb.PrepareDeployTransactionRequest:
		log.L(ctx).Infof("Received PrepareDeployTransactionRequest")

		params, err := d.validateDeploy(m.Transaction)
		if err != nil {
			return nil, err
		}
		notary := m.ResolvedVerifiers[0].Verifier

		return &pb.PrepareDeployTransactionResponse{
			Transaction: &pb.BaseLedgerTransaction{
				FunctionName: "deploy",
				ParamsJson: fmt.Sprintf(`{
					"txId": "%s",
					"notary": "%s"
				}`, m.Transaction.TransactionId, notary),
			},
			SigningAddress: params.Notary,
		}, nil

	case *pb.InitTransactionRequest:
		log.L(ctx).Infof("Received InitTransactionRequest")

		tx, err := d.validateTransaction(ctx, m.Transaction)
		if err != nil {
			return nil, err
		}

		requiredVerifiers := []*pb.ResolveVerifierRequest{{
			Lookup:    tx.domainConfig.Notary,
			Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
		}}

		switch params := tx.params.(type) {
		case NotoMintParams:
			requiredVerifiers = append(requiredVerifiers, &pb.ResolveVerifierRequest{
				Lookup:    params.To,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			})
		case NotoTransferParams:
			requiredVerifiers = append(requiredVerifiers, &pb.ResolveVerifierRequest{
				Lookup:    params.From,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			}, &pb.ResolveVerifierRequest{
				Lookup:    params.To,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			})
		default:
			return nil, fmt.Errorf("failed to parse parameters: %v", tx.params)
		}

		return &pb.InitTransactionResponse{
			RequiredVerifiers: requiredVerifiers,
		}, nil

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")

		tx, err := d.validateTransaction(ctx, m.Transaction)
		if err != nil {
			return nil, err
		}
		// TODO: use tx.domainConfig.Notary instead of hard-coding below

		var assembled *assemblyResult
		switch params := tx.params.(type) {
		case NotoMintParams:
			assembled, err = d.assembleMint(&params)
		case NotoTransferParams:
			assembled, err = d.assembleTransfer(ctx, &params)
		default:
			err = fmt.Errorf("failed to parse parameters: %v", tx.params)
		}
		if err != nil {
			return nil, err
		}

		encodedTransfer, err := d.encodeTransferData(ctx, tx.contractAddress, assembled.spentCoins, assembled.newCoins)
		if err != nil {
			return nil, err
		}

		return &pb.AssembleTransactionResponse{
			AssemblyResult: pb.AssembleTransactionResponse_OK,
			AssembledTransaction: &pb.AssembledTransaction{
				SpentStates: assembled.spentStates,
				NewStates:   assembled.newStates,
			},
			AttestationPlan: []*pb.AttestationRequest{
				{
					Name:            "sender",
					AttestationType: pb.AttestationType_SIGN,
					Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
					Payload:         encodedTransfer,
					Parties: []string{
						m.Transaction.From,
					},
				},
				{
					Name:            "notary",
					AttestationType: pb.AttestationType_ENDORSE,
					Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
					Parties: []string{
						"notary", // TODO: why can't we pass notary address here?
					},
				},
			},
		}, nil

	case *pb.EndorseTransactionRequest:
		log.L(ctx).Infof("Received EndorseTransactionRequest")

		tx, err := d.validateTransaction(ctx, m.Transaction)
		if err != nil {
			return nil, err
		}

		inCoins := make([]*NotoCoin, len(m.Inputs))
		inTotal := big.NewInt(0)
		for i, input := range m.Inputs {
			if input.SchemaId != d.coinSchema.Id {
				return nil, fmt.Errorf("unknown schema ID: %s", input.SchemaId)
			}
			if inCoins[i], err = d.makeCoin(input.StateDataJson); err != nil {
				return nil, fmt.Errorf("invalid input[%d] (%s): %s", i, input.HashId, err)
			}
			inTotal = inTotal.Add(inTotal, inCoins[i].Amount.BigInt())
		}
		outCoins := make([]*NotoCoin, len(m.Outputs))
		outTotal := big.NewInt(0)
		for i, output := range m.Outputs {
			if output.SchemaId != d.coinSchema.Id {
				return nil, fmt.Errorf("unknown schema ID: %s", output.SchemaId)
			}
			if outCoins[i], err = d.makeCoin(output.StateDataJson); err != nil {
				return nil, fmt.Errorf("invalid output[%d] (%s): %s", i, output.HashId, err)
			}
			outTotal = outTotal.Add(outTotal, outCoins[i].Amount.BigInt())
		}

		switch params := tx.params.(type) {
		case NotoMintParams:
			if len(inCoins) > 0 {
				return nil, fmt.Errorf("invalid inputs to 'mint': %v", inCoins)
			}
			if outTotal.Cmp(params.Amount.BigInt()) != 0 {
				return nil, fmt.Errorf("invalid amount for 'mint'")
			}
		case NotoTransferParams:
			if inTotal.Cmp(outTotal) != 0 {
				return nil, fmt.Errorf("invalid amount for 'transfer'")
			}
		default:
			err = fmt.Errorf("failed to parse parameters: %v", tx.params)
		}
		if err != nil {
			return nil, err
		}

		var senderSignature *pb.AttestationResult
		for _, ar := range m.Signatures {
			if ar.AttestationType == pb.AttestationType_SIGN &&
				ar.Name == "sender" &&
				ar.Verifier.Algorithm == api.Algorithm_ECDSA_SECP256K1_PLAINBYTES {
				senderSignature = ar
				break
			}
		}
		if senderSignature == nil {
			return nil, fmt.Errorf("did not find 'sender' attestation result")
		}

		encodedTransfer, err := d.encodeTransferData(ctx, tx.contractAddress, inCoins, outCoins)
		if err != nil {
			return nil, err
		}
		signingAddress, err := d.recoverSignature(ctx, encodedTransfer, senderSignature.Payload)
		if err != nil {
			return nil, err
		}
		if signingAddress.String() != senderSignature.Verifier.Verifier {
			return nil, fmt.Errorf("sender signature does not match")
		}

		return &pb.EndorseTransactionResponse{
			EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
		}, nil

	case *pb.PrepareTransactionRequest:
		log.L(ctx).Infof("Received PrepareTransactionRequest")

		inputs := make([]string, len(m.Transaction.SpentStates))
		for i, state := range m.Transaction.SpentStates {
			inputs[i] = state.HashId
		}
		outputs := make([]string, len(m.Transaction.NewStates))
		for i, state := range m.Transaction.NewStates {
			outputs[i] = state.HashId
		}

		var senderSignature ethtypes.HexBytes0xPrefix
		for _, att := range m.AttestationResult {
			if att.AttestationType == pb.AttestationType_SIGN && att.Name == "sender" {
				senderSignature = att.Payload
			}
		}

		params := map[string]interface{}{
			"inputs":    inputs,
			"outputs":   outputs,
			"signature": senderSignature,
			"data":      m.Transaction.TransactionId,
		}
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}

		return &pb.PrepareTransactionResponse{
			Transaction: &pb.BaseLedgerTransaction{
				FunctionName: "transfer", // TODO: can we have more than one method on base ledger?
				ParamsJson:   string(paramsJSON),
			},
		}, nil

	case *pb.DomainAPIError:
		log.L(ctx).Errorf("Received error: %s", m.ErrorMessage)
		return nil, nil

	default:
		log.L(ctx).Warnf("Unhandled message type: %s", reflect.TypeOf(m))
		return nil, nil
	}
}

func (d *Noto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*NotoDomainConfig, error) {
	configValues, err := NotoDomainConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config NotoDomainConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (d *Noto) validateDeploy(tx *pb.DeployTransactionSpecification) (*NotoConstructorParams, error) {
	var params NotoConstructorParams
	err := yaml.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (d *Noto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*parsedTransaction, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, err
	}

	var params interface{}
	switch functionABI.Name {
	case "mint":
		signature, err := NotoMintABI.SignatureCtx(ctx)
		if err != nil {
			return nil, err
		}
		if tx.FunctionSignature != signature {
			return nil, fmt.Errorf("unexpected signature for function: %s", functionABI.Name)
		}
		var mintParams NotoMintParams
		if err = json.Unmarshal([]byte(tx.FunctionParamsJson), &mintParams); err != nil {
			return nil, err
		}
		if mintParams.To == "" {
			return nil, fmt.Errorf("parameter 'to' is required")
		}
		if mintParams.Amount.BigInt().Sign() != 1 {
			return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
		}
		params = mintParams

	case "transfer":
		signature, err := NotoTransferABI.SignatureCtx(ctx)
		if err != nil {
			return nil, err
		}
		if tx.FunctionSignature != signature {
			return nil, fmt.Errorf("unexpected signature for function: %s", functionABI.Name)
		}
		var transferParams NotoTransferParams
		if err = json.Unmarshal([]byte(tx.FunctionParamsJson), &transferParams); err != nil {
			return nil, err
		}
		if transferParams.From == "" {
			return nil, fmt.Errorf("parameter 'from' is required")
		}
		if transferParams.To == "" {
			return nil, fmt.Errorf("parameter 'to' is required")
		}
		if transferParams.Amount.BigInt().Sign() != 1 {
			return nil, fmt.Errorf("parameter 'amount' must be greater than 0")
		}
		params = transferParams

	default:
		return nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}

	domainConfig, err := d.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, err
	}

	return &parsedTransaction{
		contractAddress: contractAddress,
		domainConfig:    domainConfig,
		params:          params,
	}, nil
}

func (d *Noto) assembleMint(params *NotoMintParams) (*assemblyResult, error) {
	outputCoins, outputStates, err := d.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}
	return &assemblyResult{
		newCoins:  outputCoins,
		newStates: outputStates,
	}, nil
}

func (d *Noto) assembleTransfer(ctx context.Context, params *NotoTransferParams) (*assemblyResult, error) {
	inputCoins, inputStates, total, err := d.prepareInputs(ctx, params.From, params.Amount)
	if err != nil {
		return nil, err
	}
	outputCoins, outputStates, err := d.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.BigInt()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.BigInt())
		returnedCoins, returnedStates, err := d.prepareOutputs(params.From, *ethtypes.NewHexInteger(remainder))
		if err != nil {
			return nil, err
		}
		outputCoins = append(outputCoins, returnedCoins...)
		outputStates = append(outputStates, returnedStates...)
	}
	return &assemblyResult{
		spentCoins:  inputCoins,
		spentStates: inputStates,
		newCoins:    outputCoins,
		newStates:   outputStates,
	}, nil
}

func (d *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, d.chainID)
}
