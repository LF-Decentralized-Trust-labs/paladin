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
	"reflect"
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
	To     string `json:"to"`
	Amount string `json:"amount"`
}

var NotoMintABI = &abi.Entry{
	Name: "mint",
	Type: abi.Function,
	Inputs: abi.ParameterArray{
		{Name: "to", Type: "string"},
		{Name: "amount", Type: "uint256"},
	},
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
	Salt   string `json:"salt"`
	Owner  string `json:"owner"`
	Amount string `json:"amount"`
}

var NotoCoinABI = &abi.Parameter{
	Type:         "tuple",
	InternalType: "struct NotoCoin",
	Components: abi.ParameterArray{
		{Name: "salt", Type: "bytes32"},
		{Name: "owner", Type: "string"},
		{Name: "amount", Type: "uint256"},
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
		err = d.handleMessage(ctx, in)
		if err != nil {
			log.L(ctx).Errorf("Error handling message - terminating handler loop: %v", err)
			return
		}
	}
}

func (d *Noto) handleMessage(ctx context.Context, message *pb.Message) error {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return err
	}

	inflight := d.replies.getInflight(message.CorrelationId)
	if inflight != nil {
		inflight.done <- message
		return nil
	}

	switch m := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.L(ctx).Infof("Received ConfigureDomainRequest")

		var config Config
		err := yaml.Unmarshal([]byte(m.ConfigYaml), &config)
		if err != nil {
			return err
		}
		factoryJSON, err := json.Marshal(d.Factory.ABI)
		if err != nil {
			return err
		}
		notoJSON, err := json.Marshal(d.Contract.ABI)
		if err != nil {
			return err
		}
		constructorJSON, err := json.Marshal(NotoConstructorABI)
		if err != nil {
			return err
		}
		schemaJSON, err := json.Marshal(NotoCoinABI)
		if err != nil {
			return err
		}

		response := &pb.ConfigureDomainResponse{
			DomainConfig: &pb.DomainConfig{
				FactoryContractAddress: config.FactoryAddress,
				FactoryContractAbiJson: string(factoryJSON),
				PrivateContractAbiJson: string(notoJSON),
				ConstructorAbiJson:     string(constructorJSON),
				AbiStateSchemasJson:    []string{string(schemaJSON)},
			},
		}
		return d.sendReply(ctx, message, response)

	case *pb.InitDomainRequest:
		log.L(ctx).Infof("Received InitDomainRequest")
		d.domainID = m.DomainUuid
		d.coinSchema = m.AbiStateSchemas[0]
		return d.sendReply(ctx, message, &pb.InitDomainResponse{})

	case *pb.InitDeployTransactionRequest:
		log.L(ctx).Infof("Received InitDeployTransactionRequest")

		var params NotoConstructorParams
		err := yaml.Unmarshal([]byte(m.Transaction.ConstructorParamsJson), &params)
		if err != nil {
			return err
		}
		log.L(ctx).Infof("Deployment parameters: %+v", params)

		response := &pb.InitDeployTransactionResponse{
			RequiredVerifiers: []*pb.ResolveVerifierRequest{
				{
					Lookup:    params.Notary,
					Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
				},
			},
		}
		return d.sendReply(ctx, message, response)

	case *pb.PrepareDeployTransactionRequest:
		log.L(ctx).Infof("Received PrepareDeployTransactionRequest")

		var params NotoConstructorParams
		err := yaml.Unmarshal([]byte(m.Transaction.ConstructorParamsJson), &params)
		if err != nil {
			return err
		}
		log.L(ctx).Infof("Deployment parameters: %+v", params)
		log.L(ctx).Infof("Resolved verifiers: %+v", m.ResolvedVerifiers)
		notary := m.ResolvedVerifiers[0].Verifier

		response := &pb.PrepareDeployTransactionResponse{
			Transaction: &pb.BaseLedgerTransaction{
				FunctionName: "deploy",
				ParamsJson: fmt.Sprintf(`{
					"txId": "%s",
					"notary": "%s"
				}`, m.Transaction.TransactionId, notary),
			},
			SigningAddress: params.Notary,
		}
		return d.sendReply(ctx, message, response)

	case *pb.InitTransactionRequest:
		log.L(ctx).Infof("Received InitTransactionRequest")

		response := &pb.InitTransactionResponse{
			RequiredVerifiers: []*pb.ResolveVerifierRequest{},
		}
		return d.sendReply(ctx, message, response)

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")

		var functionABI abi.Entry
		err := json.Unmarshal([]byte(m.Transaction.FunctionAbiJson), &functionABI)
		if err != nil {
			return err
		}

		var assembled *pb.AssembledTransaction
		switch functionABI.Name {
		case "mint":
			assembled, err = d.assembleMint(m.Transaction.FunctionParamsJson)
		default:
			err = fmt.Errorf("Unsupported method: %s", functionABI.Name)
		}
		if err != nil {
			return err
		}

		_, err = d.decodeDomainConfig(ctx, m.Transaction.ContractConfig)
		if err != nil {
			return err
		}
		// TODO: use config.Notary instead of hard-coding below

		response := &pb.AssembleTransactionResponse{
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
		}
		return d.sendReply(ctx, message, response)

	case *pb.EndorseTransactionRequest:
		log.L(ctx).Infof("Received EndorseTransactionRequest")

		response := &pb.EndorseTransactionResponse{
			EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
		}
		return d.sendReply(ctx, message, response)

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
			return err
		}

		response := &pb.PrepareTransactionResponse{
			Transaction: &pb.BaseLedgerTransaction{
				FunctionName: "transfer", // TODO: can we have more than one method on base ledger?
				ParamsJson:   string(paramsJSON),
			},
		}
		return d.sendReply(ctx, message, response)

	case *pb.DomainAPIError:
		log.L(ctx).Errorf("Received error: %s", m.ErrorMessage)
		return nil

	default:
		log.L(ctx).Errorf("Unknown type: %s", reflect.TypeOf(m))
		return nil
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

func (d *Noto) assembleMint(params string) (*pb.AssembledTransaction, error) {
	var functionParams NotoMintParams
	err := json.Unmarshal([]byte(params), &functionParams)
	if err != nil {
		return nil, err
	}

	newCoin := &NotoCoin{
		Salt:   types.RandHex(32),
		Owner:  functionParams.To,
		Amount: functionParams.Amount,
	}
	newCoinJSON, err := json.Marshal(newCoin)
	if err != nil {
		return nil, err
	}

	return &pb.AssembledTransaction{
		NewStates: []*pb.NewState{
			{
				SchemaId:      d.coinSchema.Id,
				StateDataJson: string(newCoinJSON),
			},
		},
	}, nil
}

func (d *Noto) FindCoins(ctx context.Context, query string) ([]*NotoCoin, error) {
	req := &pb.FindAvailableStatesRequest{
		DomainUuid: d.domainID,
		SchemaId:   d.coinSchema.Id,
		QueryJson:  query,
	}

	res := &pb.FindAvailableStatesResponse{}
	err := requestReply(ctx, d.replies, "from-domain", *d.dest, req, &res)
	if err != nil {
		return nil, err
	}

	coins := make([]*NotoCoin, len(res.States))
	for i, state := range res.States {
		coins[i] = &NotoCoin{}
		if err := json.Unmarshal([]byte(state.DataJson), &coins[i]); err != nil {
			return nil, err
		}
	}
	return coins, err
}
