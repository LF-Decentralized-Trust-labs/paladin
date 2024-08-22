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

//go:embed abis/NotoSelfSubmitFactory.json
var notoSelfSubmitFactoryJSON []byte // From "gradle copySolidity"

//go:embed abis/NotoSelfSubmit.json
var notoSelfSubmitJSON []byte // From "gradle copySolidity"

var (
	fromDomain = "from-domain"
)

type Config struct {
	FactoryAddress string `json:"factoryAddress" yaml:"factoryAddress"`
	Variant        string `json:"variant" yaml:"variant"`
}

type SolidityBuild struct {
	ABI      abi.ABI                   `json:"abi"`
	Bytecode ethtypes.HexBytes0xPrefix `json:"bytecode"`
}

type Noto struct {
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

type NotoDomainConfig struct {
	NotaryLookup  string `json:"notaryLookup"`
	NotaryAddress string `json:"notaryAddress"`
}

var NotoDomainConfigABI = &abi.ParameterArray{
	{Name: "notaryLookup", Type: "string"},
	{Name: "notaryAddress", Type: "address"},
}

type NotoDeployParams struct {
	TransactionID string                    `json:"transactionId"`
	Notary        string                    `json:"notary"`
	Data          ethtypes.HexBytes0xPrefix `json:"data"`
}

type parsedTransaction struct {
	transaction     *pb.TransactionSpecification
	functionABI     *abi.Entry
	contractAddress *ethtypes.Address0xHex
	domainConfig    *NotoDomainConfig
	params          interface{}
}

type gatheredCoins struct {
	inCoins   []*NotoCoin
	inStates  []*pb.StateRef
	inTotal   *big.Int
	outCoins  []*NotoCoin
	outStates []*pb.StateRef
	outTotal  *big.Int
}

func loadBuild(buildOutput []byte) SolidityBuild {
	var build SolidityBuild
	err := json.Unmarshal(buildOutput, &build)
	if err != nil {
		panic(err)
	}
	return build
}

func New(ctx context.Context, addr string) (*Noto, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}
	noto := &Noto{
		conn:   conn,
		client: pb.NewKataMessageServiceClient(conn),
	}
	noto.replies = &replyTracker{
		inflight: make(map[string]*inflightRequest),
		client:   noto.client,
	}
	noto.Interface = noto.getInterface()
	return noto, noto.waitForReady(ctx, 2*time.Second)
}

func (n *Noto) waitForReady(ctx context.Context, deadline time.Duration) error {
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

func (n *Noto) Close() error {
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

func (n *Noto) Listen(ctx context.Context, dest string) error {
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

func (n *Noto) sendReply(ctx context.Context, message *pb.Message, reply proto.Message) error {
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

func (n *Noto) handler(ctx context.Context) {
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

func (n *Noto) handleMessage(ctx context.Context, message *pb.Message) (reply proto.Message, err error) {
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

func (n *Noto) configure(req *pb.ConfigureDomainRequest) (*pb.ConfigureDomainResponse, error) {
	var config Config
	err := yaml.Unmarshal([]byte(req.ConfigYaml), &config)
	if err != nil {
		return nil, err
	}

	n.config = &config
	n.chainID = req.ChainId

	var factory SolidityBuild
	var contract SolidityBuild
	switch config.Variant {
	case "", "Noto":
		config.Variant = "Noto"
		factory = loadBuild(notoFactoryJSON)
		contract = loadBuild(notoJSON)
	case "NotoSelfSubmit":
		factory = loadBuild(notoSelfSubmitFactoryJSON)
		contract = loadBuild(notoSelfSubmitJSON)
	default:
		return nil, fmt.Errorf("unrecognized variant: %s", config.Variant)
	}

	factoryJSON, err := json.Marshal(factory.ABI)
	if err != nil {
		return nil, err
	}
	notoJSON, err := json.Marshal(contract.ABI)
	if err != nil {
		return nil, err
	}
	constructorJSON, err := json.Marshal(n.Interface["constructor"].ABI)
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
}

func (n *Noto) init(req *pb.InitDomainRequest) (*pb.InitDomainResponse, error) {
	n.domainID = req.DomainUuid
	n.coinSchema = req.AbiStateSchemas[0]
	log.L(context.TODO()).Infof("Received schema: %s", n.coinSchema)
	return &pb.InitDomainResponse{}, nil
}

func (n *Noto) initDeploy(params *NotoConstructorParams) (*pb.InitDeployTransactionResponse, error) {
	return &pb.InitDeployTransactionResponse{
		RequiredVerifiers: []*pb.ResolveVerifierRequest{
			{
				Lookup:    params.Notary,
				Algorithm: api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
			},
		},
	}, nil
}

func (n *Noto) prepareDeploy(ctx context.Context, req *pb.PrepareDeployTransactionRequest) (*pb.PrepareDeployTransactionResponse, error) {
	config := &NotoDomainConfig{
		NotaryLookup:  req.ResolvedVerifiers[0].Lookup,
		NotaryAddress: req.ResolvedVerifiers[0].Verifier,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	data, err := NotoDomainConfigABI.EncodeABIDataJSONCtx(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	params := &NotoDeployParams{
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

func (n *Noto) decodeDomainConfig(ctx context.Context, domainConfig []byte) (*NotoDomainConfig, error) {
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

func (n *Noto) validateDeploy(tx *pb.DeployTransactionSpecification) (*NotoConstructorParams, error) {
	var params NotoConstructorParams
	err := yaml.Unmarshal([]byte(tx.ConstructorParamsJson), &params)
	return &params, err
}

func (n *Noto) validateTransaction(ctx context.Context, tx *pb.TransactionSpecification) (*parsedTransaction, error) {
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

func (n *Noto) recoverSignature(ctx context.Context, payload ethtypes.HexBytes0xPrefix, signature []byte) (*ethtypes.Address0xHex, error) {
	sig, err := secp256k1.DecodeCompactRSV(ctx, signature)
	if err != nil {
		return nil, err
	}
	return sig.RecoverDirect(payload, n.chainID)
}

func (h *domainHandler) parseCoinList(label string, states []*pb.EndorsableState) ([]*NotoCoin, []*pb.StateRef, *big.Int, error) {
	var err error
	coins := make([]*NotoCoin, len(states))
	refs := make([]*pb.StateRef, len(states))
	total := big.NewInt(0)
	for i, input := range states {
		if input.SchemaId != h.noto.coinSchema.Id {
			return nil, nil, nil, fmt.Errorf("unknown schema ID: %s", input.SchemaId)
		}
		if coins[i], err = h.noto.makeCoin(input.StateDataJson); err != nil {
			return nil, nil, nil, fmt.Errorf("invalid %s[%d] (%s): %s", label, i, input.HashId, err)
		}
		refs[i] = &pb.StateRef{
			SchemaId: input.SchemaId,
			HashId:   input.HashId,
		}
		total = total.Add(total, coins[i].Amount.BigInt())
	}
	return coins, refs, total, nil
}

func (h *domainHandler) gatherCoins(inputs, outputs []*pb.EndorsableState) (*gatheredCoins, error) {
	inCoins, inStates, inTotal, err := h.parseCoinList("input", inputs)
	if err != nil {
		return nil, err
	}
	outCoins, outStates, outTotal, err := h.parseCoinList("output", outputs)
	if err != nil {
		return nil, err
	}
	return &gatheredCoins{
		inCoins:   inCoins,
		inStates:  inStates,
		inTotal:   inTotal,
		outCoins:  outCoins,
		outStates: outStates,
		outTotal:  outTotal,
	}, nil
}
