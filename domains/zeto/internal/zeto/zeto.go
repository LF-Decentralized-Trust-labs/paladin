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

package zeto

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	pb "github.com/kaleido-io/paladin/kata/pkg/proto"
	"github.com/kaleido-io/paladin/kata/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v2"
)

//go:embed abis/Commonlib.json
var commonLibJSON []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_Anon.json
var Groth16Verifier_Anon []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_CheckHashesValue.json
var Groth16Verifier_CheckHashesValue []byte // From "gradle copySolidity"

//go:embed abis/Groth16Verifier_CheckInputsOutputsValue.json
var Groth16Verifier_CheckInputsOutputsValue []byte // From "gradle copySolidity"

//go:embed abis/ZetoSampleFactory.json
var zetoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/ZetoSample.json
var zetoJSON []byte // From "gradle copySolidity"

var (
	fromDomain = "from-domain"
)

type Config struct {
	FactoryAddress string            `json:"factoryAddress" yaml:"factoryAddress"`
	Libraries      map[string]string `json:"libraries" yaml:"libraries"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type SolidityBuildWithLinks struct {
	ABI            abi.ABI                                       `json:"abi"`
	Bytecode       string                                        `json:"bytecode"`
	LinkReferences map[string]map[string][]SolidityLinkReference `json:"linkReferences"`
}

type SolidityLinkReference struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

type Zeto struct {
	Interface DomainInterface

	config       *Config
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

type ZetoDomainConfig struct {
}

var ZetoDomainConfigABI = &abi.ParameterArray{}

type ZetoDeployParams struct {
	TransactionID    string                    `json:"transactionId"`
	Data             ethtypes.HexBytes0xPrefix `json:"data"`
	Verifier         string                    `json:"_verifier"`
	DepositVerifier  string                    `json:"_depositVerifier"`
	WithdrawVerifier string                    `json:"_withdrawVerifier"`
}

type parsedTransaction struct {
	transaction     *pb.TransactionSpecification
	functionABI     *abi.Entry
	contractAddress *ethtypes.Address0xHex
	domainConfig    *ZetoDomainConfig
	params          interface{}
}

func loadBuild(buildOutput []byte) SolidityBuild {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	return build
}

func loadBuildLinked(buildOutput []byte, libraries map[string]string) SolidityBuild {
	var build SolidityBuildWithLinks
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	bytecode, err := linkBytecode(build, libraries)
	if err != nil {
		panic(err)
	}
	return SolidityBuild{
		ABI:      build.ABI,
		Bytecode: bytecode,
	}
}

// linkBytecode: performs linking by replacing placeholders with deployed addresses
// Based on a workaround from Hardhat team here:
// https://github.com/nomiclabs/hardhat/issues/611#issuecomment-638891597
func linkBytecode(artifact SolidityBuildWithLinks, libraries map[string]string) (ethtypes.HexBytes0xPrefix, error) {
	bytecode := artifact.Bytecode
	for _, fileReferences := range artifact.LinkReferences {
		for libName, fixups := range fileReferences {
			addr, found := libraries[libName]
			if !found {
				continue
			}
			for _, fixup := range fixups {
				start := 2 + fixup.Start*2
				end := start + fixup.Length*2
				bytecode = bytecode[0:start] + addr[2:] + bytecode[end:]
			}
		}
	}
	return hex.DecodeString(strings.TrimPrefix(bytecode, "0x"))
}

func New(ctx context.Context, addr string) (*Zeto, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}
	zeto := &Zeto{
		conn:   conn,
		client: pb.NewKataMessageServiceClient(conn),
	}
	zeto.replies = &replyTracker{
		inflight: make(map[string]*inflightRequest),
		client:   zeto.client,
	}
	zeto.Interface = zeto.getInterface()
	return zeto, zeto.waitForReady(ctx, 2*time.Second)
}

func (z *Zeto) waitForReady(ctx context.Context, deadline time.Duration) error {
	status, err := z.client.Status(ctx, &pb.StatusRequest{})
	end := time.Now().Add(deadline)
	for !status.GetOk() {
		time.Sleep(time.Second)
		if time.Now().After(end) {
			return fmt.Errorf("server was not ready after %s", deadline)
		}
		status, err = z.client.Status(ctx, &pb.StatusRequest{})
	}
	if err != nil {
		return err
	}
	if !status.GetOk() {
		return fmt.Errorf("got non-OK status from server")
	}
	return nil
}

func (z *Zeto) Close() error {
	if z.stream != nil {
		if err := z.stream.CloseSend(); err != nil {
			return err
		}
		z.done <- true
		z.stopListener()
	}
	if z.conn != nil {
		if err := z.conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (z *Zeto) Listen(ctx context.Context, dest string) error {
	z.dest = &dest
	z.done = make(chan bool, 1)

	var err error
	var listenerContext context.Context

	listenerContext, z.stopListener = context.WithCancel(ctx)
	z.stream, err = z.client.Listen(listenerContext, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return fmt.Errorf("failed to listen for domain events: %v", err)
	}

	handlerCtx := log.WithLogField(ctx, "role", "handler")
	go z.handler(handlerCtx)
	return nil
}

func (z *Zeto) sendReply(ctx context.Context, message *pb.Message, reply proto.Message) error {
	body, err := anypb.New(reply)
	if err == nil {
		_, err = z.client.SendMessage(ctx, &pb.Message{
			Destination:   *message.ReplyTo,
			CorrelationId: &message.Id,
			Body:          body,
			ReplyTo:       z.dest,
		})
	}
	return err
}

func (z *Zeto) handler(ctx context.Context) {
	for {
		in, err := z.stream.Recv()
		select {
		case <-z.done:
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
			reply, err := z.handleMessage(ctx, in)
			if err != nil {
				reply = &pb.DomainAPIError{ErrorMessage: err.Error()}
				err = nil
			}
			if reply != nil {
				if err = z.sendReply(ctx, in, reply); err != nil {
					log.L(ctx).Errorf("Error sending message reply: %s", err)
				}
			}
		}()
	}
}

func (z *Zeto) handleMessage(ctx context.Context, message *pb.Message) (reply proto.Message, err error) {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return nil, err
	}

	inflight := z.replies.getInflight(message.CorrelationId)
	if inflight != nil {
		inflight.done <- message
		return nil, nil
	}

	switch req := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.L(ctx).Infof("Received ConfigureDomainRequest")
		return z.configure(req)

	case *pb.InitDomainRequest:
		log.L(ctx).Infof("Received InitDomainRequest")
		return z.init(req)

	case *pb.InitDeployTransactionRequest:
		log.L(ctx).Infof("Received InitDeployTransactionRequest")
		_, err := z.validateDeploy(req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.initDeploy()

	case *pb.PrepareDeployTransactionRequest:
		log.L(ctx).Infof("Received PrepareDeployTransactionRequest")
		params, err := z.validateDeploy(req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.prepareDeploy(params, req)

	case *pb.InitTransactionRequest:
		log.L(ctx).Infof("Received InitTransactionRequest")
		tx, err := z.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.Interface[tx.functionABI.Name].handler.Init(ctx, tx, req)

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")
		tx, err := z.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.Interface[tx.functionABI.Name].handler.Assemble(ctx, tx, req)

	case *pb.EndorseTransactionRequest:
		log.L(ctx).Infof("Received EndorseTransactionRequest")
		tx, err := z.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.Interface[tx.functionABI.Name].handler.Endorse(ctx, tx, req)

	case *pb.PrepareTransactionRequest:
		log.L(ctx).Infof("Received PrepareTransactionRequest")
		tx, err := z.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return z.Interface[tx.functionABI.Name].handler.Prepare(ctx, tx, req)

	case *pb.DomainAPIError:
		log.L(ctx).Errorf("Received error: %s", req.ErrorMessage)
		return nil, nil

	default:
		log.L(ctx).Warnf("Unhandled message type: %s", reflect.TypeOf(req))
		return nil, nil
	}
}

func (z *Zeto) configure(req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config Config
	err := yaml.Unmarshal([]byte(req.ConfigYaml), &config)
	if err != nil {
		return nil, err
	}

	z.config = &config
	z.chainID = req.ChainId

	factory := loadBuildLinked(zetoFactoryJSON, config.Libraries)
	contract := loadBuildLinked(zetoJSON, config.Libraries)

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	zetoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(z.Interface["constructor"].ABI)
	if err != nil {
		return nil, err
	}
	schemaJSON, err := json.Marshal(ZetoCoinABI)
	if err != nil {
		return nil, err
	}

	return &pb.ConfigureDomainResponse{
		DomainConfig: &pb.DomainConfig{
			FactoryContractAddress: config.FactoryAddress,
			FactoryContractAbiJson: string(factoryJSON),
			PrivateContractAbiJson: string(zetoJSON),
			ConstructorAbiJson:     string(constructorJSON),
			AbiStateSchemasJson:    []string{string(schemaJSON)},
		},
	}, nil
}

func (z *Zeto) init(req *pb.InitDomainRequest) (*pb.InitDomainResponse, error) {
	z.domainID = req.DomainUuid
	z.coinSchema = req.AbiStateSchemas[0]
	return &pb.InitDomainResponse{}, nil
}

func (z *Zeto) initDeploy() (*pb.InitDeployTransactionResponse, error) {
	return &pb.InitDeployTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			// TODO: should we resolve anything?
		},
	}, nil
}

func (z *Zeto) prepareDeploy(params *ZetoConstructorParams, req *pb.PrepareDeployTransactionRequest) (*pb.PrepareDeployTransactionResponse, error) {
	deployParams := &ZetoDeployParams{
		TransactionID:    req.Transaction.TransactionId,
		Data:             ethtypes.HexBytes0xPrefix(""),
		DepositVerifier:  params.DepositVerifier,
		WithdrawVerifier: params.WithdrawVerifier,
		Verifier:         params.Verifier,
	}
	paramsJSON, err := json.Marshal(deployParams)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareDeployTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "deploy",
			ParamsJson:   string(paramsJSON),
		},
		SigningAddress: params.From,
	}, nil
}

func (z *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*ZetoDomainConfig, error) {
	configValues, err := ZetoDomainConfigABI.DecodeABIDataCtx(ctx, domainConfig, 0)
	if err != nil {
		return nil, err
	}
	configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
	if err != nil {
		return nil, err
	}
	var config ZetoDomainConfig
	err = json.Unmarshal(configJSON, &config)
	return &config, err
}

func (z *Zeto) validateDeploy(tx *pb.DeployTransactionSpecification) (*ZetoConstructorParams, error) {
	var params ZetoConstructorParams
	err := yaml.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (z *Zeto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*parsedTransaction, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, err
	}

	parser, found := z.Interface[functionABI.Name]
	if !found {
		return nil, fmt.Errorf("unknown function: %s", functionABI.Name)
	}
	params, err := parser.handler.ValidateParams(tx.FunctionParamsJson)
	if err != nil {
		return nil, err
	}

	signature, err := parser.ABI.SignatureCtx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.FunctionSignature != signature {
		return nil, fmt.Errorf("unexpected signature for function: %s", functionABI.Name)
	}

	domainConfig, err := z.decodeDomainConfig(ctx, tx.ContractConfig)
	if err != nil {
		return nil, err
	}

	contractAddress, err := ethtypes.NewAddress(tx.ContractAddress)
	if err != nil {
		return nil, err
	}

	return &parsedTransaction{
		transaction:     tx,
		functionABI:     &functionABI,
		contractAddress: contractAddress,
		domainConfig:    domainConfig,
		params:          params,
	}, nil
}
