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
	"strconv"
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
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
	domainID     string
	coinSchema   *pb.StateSchema
	replies      *replyTracker
}

type NotoConstructorParams struct {
	Notary string `json:"notary"`
}

var NotoConstructorABI = &abi.Entry{
	Type: abi.Constructor,
	Inputs: abi.ParameterArray{
		{Name: "notary", Type: "string"},
	},
}

type NotoMintParams struct {
	To     string              `json:"to"`
	Amount ethtypes.HexInteger `json:"amount"`
}

var NotoMintABI = &abi.Entry{
	Name: "mint",
	Type: abi.Function,
	Inputs: abi.ParameterArray{
		{Name: "to", Type: "string"},
		{Name: "amount", Type: "uint256"},
	},
}

type NotoTransferParams struct {
	From   string              `json:"from"`
	To     string              `json:"to"`
	Amount ethtypes.HexInteger `json:"amount"`
}

var NotoTransferABI = &abi.Entry{
	Name: "transfer",
	Type: abi.Function,
	Inputs: abi.ParameterArray{
		{Name: "from", Type: "string"},
		{Name: "to", Type: "string"},
		{Name: "amount", Type: "uint256"},
	},
}

var NotoABI = &abi.ABI{
	NotoConstructorABI,
	NotoMintABI,
	NotoTransferABI,
}

type NotoDomainConfig struct {
	Notary string `json:"notary"`
}

var NotoDomainConfigABI = &abi.ParameterArray{
	{Name: "notary", Type: "address"},
}

type NotoCoin struct {
	Salt   string              `json:"salt"`
	Owner  string              `json:"owner"`
	Amount ethtypes.HexInteger `json:"amount"`
}

var NotoCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string", Indexed: true},
		{Name: "amount", Type: "uint256", Indexed: true},
	},
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

		var params NotoConstructorParams
		err := yaml.Unmarshal([]byte(m.Transaction.ConstructorParamsJson), &params)
		if err != nil {
			return nil, err
		}
		log.L(ctx).Infof("Deployment parameters: %+v", params)

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

		var params NotoConstructorParams
		err := yaml.Unmarshal([]byte(m.Transaction.ConstructorParamsJson), &params)
		if err != nil {
			return nil, err
		}
		log.L(ctx).Infof("Deployment parameters: %+v", params)
		log.L(ctx).Infof("Resolved verifiers: %+v", m.ResolvedVerifiers)
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

		return &pb.InitTransactionResponse{
			RequiredVerifiers: []*pb.ResolveVerifierRequest{},
		}, nil

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")

		var functionABI abi.Entry
		err := json.Unmarshal([]byte(m.Transaction.FunctionAbiJson), &functionABI)
		if err != nil {
			return nil, err
		}

		var assembled *pb.AssembledTransaction
		switch functionABI.Name {
		case "mint":
			var mintParams NotoMintParams
			err = json.Unmarshal([]byte(m.Transaction.FunctionParamsJson), &mintParams)
			if err == nil {
				assembled, err = d.assembleMint(&mintParams)
			}
		case "transfer":
			var transferParams NotoTransferParams
			err = json.Unmarshal([]byte(m.Transaction.FunctionParamsJson), &transferParams)
			if err == nil {
				assembled, err = d.assembleTransfer(ctx, &transferParams)
			}
		default:
			err = fmt.Errorf("Unsupported method: %s", functionABI.Name)
		}
		if err != nil {
			return nil, err
		}

		_, err = d.decodeDomainConfig(ctx, m.Transaction.ContractConfig)
		if err != nil {
			return nil, err
		}
		// TODO: use config.Notary instead of hard-coding below

		return &pb.AssembleTransactionResponse{
			AssemblyResult:       pb.AssembleTransactionResponse_OK,
			AssembledTransaction: assembled,
			AttestationPlan: []*pb.AttestationRequest{
				{
					Name:            "signer",
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

		params := map[string]interface{}{
			"inputs":    inputs,
			"outputs":   outputs,
			"signature": "0x",
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

func (d *Noto) makeCoin(state *pb.StoredState) (*NotoCoin, error) {
	coin := &NotoCoin{}
	err := json.Unmarshal([]byte(state.DataJson), &coin)
	return coin, err
}

func (d *Noto) makeState(coin *NotoCoin) (*pb.NewState, error) {
	coinJSON, err := json.Marshal(coin)
	if err != nil {
		return nil, err
	}
	return &pb.NewState{
		SchemaId:      d.coinSchema.Id,
		StateDataJson: string(coinJSON),
	}, nil
}

func (d *Noto) prepareInputs(ctx context.Context, owner string, amount ethtypes.HexInteger) ([]*pb.StateRef, *big.Int, error) {
	var lastStateTimestamp int64
	total := big.NewInt(0)
	stateRefs := []*pb.StateRef{}
	for {
		// Simple oldest coin first algorithm
		// TODO: make this configurable
		// TODO: why is filters.QueryJSON not a public interface?
		query := fmt.Sprintf(`
			"limit": 10,
			"sort": [".created"],
			"eq": [{
				"field": "owner",
				"value": "%s"
			}]
		`, owner)
		if lastStateTimestamp > 0 {
			query += fmt.Sprintf(`,
				"gt": [{
					"field": ".created",
					"value": "%s"
				}]
			`, strconv.FormatInt(lastStateTimestamp, 10))
		}
		query = "{" + query + "}"

		states, err := d.findAvailableStates(ctx, query)
		if err != nil {
			return nil, nil, err
		}
		if len(states) == 0 {
			return nil, nil, fmt.Errorf("insufficient funds (available=%s)", total.Text(10))
		}
		for _, state := range states {
			lastStateTimestamp = state.StoredAt
			coin, err := d.makeCoin(state)
			if err != nil {
				return nil, nil, fmt.Errorf("coin %s is invalid: %s", state.HashId, err)
			}
			total = total.Add(total, coin.Amount.BigInt())
			stateRefs = append(stateRefs, &pb.StateRef{
				HashId:   state.HashId,
				SchemaId: state.SchemaId,
			})
			if total.Cmp(amount.BigInt()) >= 0 {
				return stateRefs, total, nil
			}
		}
	}
}

func (d *Noto) prepareOutputs(owner string, amount ethtypes.HexInteger) ([]*pb.NewState, error) {
	// Always produce a single coin for the entire output amount
	// TODO: make this configurable
	newCoin := &NotoCoin{
		Salt:   types.RandHex(32),
		Owner:  owner,
		Amount: amount,
	}
	newState, err := d.makeState(newCoin)
	if err != nil {
		return nil, err
	}
	return []*pb.NewState{newState}, nil
}

func (d *Noto) assembleMint(params *NotoMintParams) (*pb.AssembledTransaction, error) {
	outputs, err := d.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}
	return &pb.AssembledTransaction{
		NewStates: outputs,
	}, nil
}

func (d *Noto) assembleTransfer(ctx context.Context, params *NotoTransferParams) (*pb.AssembledTransaction, error) {
	inputs, total, err := d.prepareInputs(ctx, params.From, params.Amount)
	if err != nil {
		return nil, err
	}
	outputs, err := d.prepareOutputs(params.To, params.Amount)
	if err != nil {
		return nil, err
	}
	if total.Cmp(params.Amount.BigInt()) == 1 {
		remainder := big.NewInt(0).Sub(total, params.Amount.BigInt())
		returnedStates, err := d.prepareOutputs(params.From, *ethtypes.NewHexInteger(remainder))
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, returnedStates...)
	}

	return &pb.AssembledTransaction{
		SpentStates: inputs,
		NewStates:   outputs,
	}, nil
}

func (d *Noto) findAvailableStates(ctx context.Context, query string) ([]*pb.StoredState, error) {
	req := &pb.FindAvailableStatesRequest{
		DomainUuid: d.domainID,
		SchemaId:   d.coinSchema.Id,
		QueryJson:  query,
	}

	res := &pb.FindAvailableStatesResponse{}
	err := requestReply(ctx, d.replies, fromDomain, *d.dest, req, &res)
	return res.States, err
}

func (d *Noto) FindCoins(ctx context.Context, query string) ([]*NotoCoin, error) {
	states, err := d.findAvailableStates(ctx, query)
	if err != nil {
		return nil, err
	}

	coins := make([]*NotoCoin, len(states))
	for i, state := range states {
		if coins[i], err = d.makeCoin(state); err != nil {
			return nil, err
		}
	}
	return coins, err
}
