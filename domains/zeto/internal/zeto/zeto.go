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
	"encoding/json"
	"fmt"
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

//go:embed abis/ZetoSampleFactory.json
var zetoFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/ZetoSample.json
var zetoJSON []byte // From "gradle copySolidity"

type Config struct {
	FactoryAddress string `json:"factoryAddress" yaml:"factoryAddress"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
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
	replies      *replyTracker
}

type ZetoDomainConfig struct {
	NotaryLookup  string `json:"notaryLookup"`
	NotaryAddress string `json:"notaryAddress"`
}

var ZetoDomainConfigABI = &abi.ParameterArray{
	{Name: "notaryLookup", Type: "string"},
	{Name: "notaryAddress", Type: "address"},
}

type ZetoDeployParams struct {
	TransactionID string                    `json:"transactionId"`
	Notary        string                    `json:"notary"`
	Data          ethtypes.HexBytes0xPrefix `json:"data"`
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

func (n *Zeto) waitForReady(ctx context.Context, deadline time.Duration) error {
	status, err := n.client.Status(ctx, &pb.StatusRequest{})
	end := time.Now().Add(deadline)
	for !status.GetOk() {
		time.Sleep(time.Second)
		if time.Now().After(end) {
			return fmt.Errorf("server was not ready after %s", deadline)
		}
		status, err = n.client.Status(ctx, &pb.StatusRequest{})
	}
	if err != nil {
		return err
	}
	if !status.GetOk() {
		return fmt.Errorf("got non-OK status from server")
	}
	return nil
}

func (n *Zeto) Close() error {
	if n.stream != nil {
		if err := n.stream.CloseSend(); err != nil {
			return err
		}
		n.done <- true
		n.stopListener()
	}
	if n.conn != nil {
		if err := n.conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (n *Zeto) Listen(ctx context.Context, dest string) error {
	n.dest = &dest
	n.done = make(chan bool, 1)

	var err error
	var listenerContext context.Context

	listenerContext, n.stopListener = context.WithCancel(ctx)
	n.stream, err = n.client.Listen(listenerContext, &pb.ListenRequest{Destination: dest})
	if err != nil {
		return fmt.Errorf("failed to listen for domain events: %v", err)
	}

	handlerCtx := log.WithLogField(ctx, "role", "handler")
	go n.handler(handlerCtx)
	return nil
}

func (n *Zeto) sendReply(ctx context.Context, message *pb.Message, reply proto.Message) error {
	body, err := anypb.New(reply)
	if err == nil {
		_, err = n.client.SendMessage(ctx, &pb.Message{
			Destination:   *message.ReplyTo,
			CorrelationId: &message.Id,
			Body:          body,
			ReplyTo:       n.dest,
		})
	}
	return err
}

func (n *Zeto) handler(ctx context.Context) {
	for {
		in, err := n.stream.Recv()
		select {
		case <-n.done:
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
			reply, err := n.handleMessage(ctx, in)
			if err != nil {
				reply = &pb.DomainAPIError{ErrorMessage: err.Error()}
				err = nil
			}
			if reply != nil {
				if err = n.sendReply(ctx, in, reply); err != nil {
					log.L(ctx).Errorf("Error sending message reply: %s", err)
				}
			}
		}()
	}
}

func (n *Zeto) handleMessage(ctx context.Context, message *pb.Message) (reply proto.Message, err error) {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return nil, err
	}

	inflight := n.replies.getInflight(message.CorrelationId)
	if inflight != nil {
		inflight.done <- message
		return nil, nil
	}

	switch req := body.(type) {
	case *pb.ConfigureDomainRequest:
		log.L(ctx).Infof("Received ConfigureDomainRequest")
		return n.configure(req)

	case *pb.InitDomainRequest:
		log.L(ctx).Infof("Received InitDomainRequest")
		return n.init(req)

	case *pb.InitDeployTransactionRequest:
		log.L(ctx).Infof("Received InitDeployTransactionRequest")
		params, err := n.validateDeploy(req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.initDeploy(params)

	case *pb.PrepareDeployTransactionRequest:
		log.L(ctx).Infof("Received PrepareDeployTransactionRequest")
		_, err := n.validateDeploy(req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.prepareDeploy(ctx, req)

	case *pb.InitTransactionRequest:
		log.L(ctx).Infof("Received InitTransactionRequest")
		tx, err := n.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.Interface[tx.functionABI.Name].handler.Init(ctx, tx, req)

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")
		tx, err := n.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.Interface[tx.functionABI.Name].handler.Assemble(ctx, tx, req)

	case *pb.EndorseTransactionRequest:
		log.L(ctx).Infof("Received EndorseTransactionRequest")
		tx, err := n.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.Interface[tx.functionABI.Name].handler.Endorse(ctx, tx, req)

	case *pb.PrepareTransactionRequest:
		log.L(ctx).Infof("Received PrepareTransactionRequest")
		tx, err := n.validateTransaction(ctx, req.Transaction)
		if err != nil {
			return nil, err
		}
		return n.Interface[tx.functionABI.Name].handler.Prepare(ctx, tx, req)

	case *pb.DomainAPIError:
		log.L(ctx).Errorf("Received error: %s", req.ErrorMessage)
		return nil, nil

	default:
		log.L(ctx).Warnf("Unhandled message type: %s", reflect.TypeOf(req))
		return nil, nil
	}
}

func (n *Zeto) configure(req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config Config
	err := yaml.Unmarshal([]byte(req.ConfigYaml), &config)
	if err != nil {
		return nil, err
	}

	n.config = &config
	n.chainID = req.ChainId

	factory := loadBuild(zetoFactoryJSON)
	contract := loadBuild(zetoJSON)

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	zetoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(n.Interface["constructor"].ABI)
	if err != nil {
		return nil, err
	}

	return &pb.ConfigureDomainResponse{
		DomainConfig: &pb.DomainConfig{
			FactoryContractAddress: config.FactoryAddress,
			FactoryContractAbiJson: string(factoryJSON),
			PrivateContractAbiJson: string(zetoJSON),
			ConstructorAbiJson:     string(constructorJSON),
			AbiStateSchemasJson:    []string{},
		},
	}, nil
}

func (n *Zeto) init(req *pb.InitDomainRequest) (*pb.InitDomainResponse, error) {
	n.domainID = req.DomainUuid
	return &pb.InitDomainResponse{}, nil
}

func (n *Zeto) initDeploy(params *ZetoConstructorParams) (*pb.InitDeployTransactionResponse, error) {
	return &pb.InitDeployTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    params.Notary,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (n *Zeto) prepareDeploy(ctx context.Context, req *pb.PrepareDeployTransactionRequest) (*pb.PrepareDeployTransactionResponse, error) {
	config := &ZetoDomainConfig{
		NotaryLookup:  req.ResolvedVerifiers[0].Lookup,
		NotaryAddress: req.ResolvedVerifiers[0].Verifier,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	data, err := ZetoDomainConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	params := &ZetoDeployParams{
		TransactionID: req.Transaction.TransactionId,
		Notary:        config.NotaryAddress,
		Data:          data,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	return &pb.PrepareDeployTransactionResponse{
		Transaction: &pb.BaseLedgerTransaction{
			FunctionName: "deploy",
			ParamsJson:   string(paramsJSON),
		},
		SigningAddress: config.NotaryLookup,
	}, nil
}

func (n *Zeto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*ZetoDomainConfig, error) {
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

func (n *Zeto) validateDeploy(tx *pb.DeployTransactionSpecification) (*ZetoConstructorParams, error) {
	var params ZetoConstructorParams
	err := yaml.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (n *Zeto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*parsedTransaction, error) {
	var functionABI abi.Entry
	err := json.Unmarshal([]byte(tx.FunctionAbiJson), &functionABI)
	if err != nil {
		return nil, err
	}

	parser, found := n.Interface[functionABI.Name]
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

	domainConfig, err := n.decodeDomainConfig(ctx, tx.ContractConfig)
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

func (n *Zeto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}
