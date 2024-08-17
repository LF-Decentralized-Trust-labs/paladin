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
}

type NotoConstructor struct {
	Notary string `json:"notary"`
}

var constructorABI = `{
	"type": "constructor",
	"inputs": [
		{
			"internalType": "string",
			"name": "notary",
			"type": "string"
		}
	]
}`

var domainConfigABI = &abi.ParameterArray{
	{Name: "notary", Type: "address"},
}

func New(ctx context.Context, addr string) (*Noto, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect gRPC: %v", err)
	}

	factory, err := loadNotoFactoryABI()
	if err != nil {
		return nil, err
	}
	contract, err := loadNotoABI()
	if err != nil {
		return nil, err
	}

	d := &Noto{
		conn:     conn,
		client:   pb.NewKataMessageServiceClient(conn),
		Factory:  factory,
		Contract: contract,
	}
	return d, d.waitForReady(ctx)
}

func loadNotoFactoryABI() (SolidityBuild, error) {
	var build SolidityBuild
	err := json.Unmarshal(notoFactoryJSON, &build)
	return build, err
}

func loadNotoABI() (SolidityBuild, error) {
	var build SolidityBuild
	err := json.Unmarshal(notoJSON, &build)
	return build, err
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

	go d.handler(ctx)
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
	handlerCtx := log.WithLogField(ctx, "role", "handler")
	for {
		in, err := d.stream.Recv()
		select {
		case <-d.done:
			return
		default:
			// do nothing
		}
		if err != nil {
			log.L(handlerCtx).Errorf("Error receiving message - terminating handler loop: %v", err)
			return
		}
		err = d.handleMessage(handlerCtx, in)
		if err != nil {
			log.L(handlerCtx).Errorf("Error handling message - terminating handler loop: %v", err)
			return
		}
	}
}

func (d *Noto) handleMessage(ctx context.Context, message *pb.Message) error {
	body, err := message.Body.UnmarshalNew()
	if err != nil {
		return err
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

		response := &pb.ConfigureDomainResponse{
			DomainConfig: &pb.DomainConfig{
				FactoryContractAddress: config.FactoryAddress,
				FactoryContractAbiJson: string(factoryJSON),
				PrivateContractAbiJson: string(notoJSON),
				ConstructorAbiJson:     constructorABI,
				AbiStateSchemasJson:    []string{},
			},
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.InitDomainRequest:
		log.L(ctx).Infof("Received InitDomainRequest")
		response := &pb.InitDomainResponse{}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.InitDeployTransactionRequest:
		log.L(ctx).Infof("Received InitDeployTransactionRequest")

		var params NotoConstructor
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
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.PrepareDeployTransactionRequest:
		log.L(ctx).Infof("Received PrepareDeployTransactionRequest")

		var params NotoConstructor
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
				ParamsJson:   `{"txId": "` + m.Transaction.TransactionId + `", "notary": "` + notary + `"}`,
			},
			SigningAddress: params.Notary,
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.InitTransactionRequest:
		log.L(ctx).Infof("Received InitTransactionRequest")

		response := &pb.InitTransactionResponse{
			RequiredVerifiers: []*pb.ResolveVerifierRequest{},
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.AssembleTransactionRequest:
		log.L(ctx).Infof("Received AssembleTransactionRequest")

		configValues, err := domainConfigABI.DecodeABIDataCtx(ctx, m.Transaction.ContractConfig, 0)
		if err != nil {
			return err
		}
		configJSON, err := types.StandardABISerializer().SerializeJSON(configValues)
		if err != nil {
			return err
		}
		var config map[string]interface{}
		err = json.Unmarshal(configJSON, &config)
		if err != nil {
			return err
		}
		// TODO: use this address instead of hard-coding below
		// notary := config["notary"].(string)

		response := &pb.AssembleTransactionResponse{
			AssemblyResult:       pb.AssembleTransactionResponse_OK,
			AssembledTransaction: &pb.AssembledTransaction{},
			AttestationPlan: []*pb.AttestationRequest{
				{
					Name:            "signer",
					AttestationType: pb.AttestationType_ENDORSE,
					Algorithm:       api.Algorithm_ECDSA_SECP256K1_PLAINBYTES,
					Parties: []string{
						"notary1", // TODO: why can't we pass notary address here?
					},
				},
			},
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.EndorseTransactionRequest:
		log.L(ctx).Infof("Received EndorseTransactionRequest")

		response := &pb.EndorseTransactionResponse{
			EndorsementResult: pb.EndorseTransactionResponse_ENDORSER_SUBMIT,
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.PrepareTransactionRequest:
		log.L(ctx).Infof("Received PrepareTransactionRequest")

		response := &pb.PrepareTransactionResponse{
			Transaction: &pb.BaseLedgerTransaction{
				FunctionName: "transfer",
				ParamsJson: `{
					"inputs": [],
					"outputs": [],
					"signature": "0x",
					"data": "0x"
				}`,
			},
		}
		if err := d.sendReply(ctx, message, response); err != nil {
			return err
		}

	case *pb.DomainAPIError:
		log.L(ctx).Errorf("Received error: %s", m.ErrorMessage)

	default:
		log.L(ctx).Errorf("Unknown type: %s", reflect.TypeOf(m))
	}

	return nil
}
