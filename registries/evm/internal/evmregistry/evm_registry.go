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

package evmregistry

import (
	"context"
	"encoding/json"

	_ "embed"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/registries/evm/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/plugintk"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
)

//go:embed abis/IdentityRegistry.json
var identityRegistryJSON []byte

var contractDetail = mustLoadIdentityRegistryContractDetail(identityRegistryJSON)

type Server interface {
	Start() error
	Stop()
}

type evmRegistry struct {
	bgCtx     context.Context
	callbacks plugintk.RegistryCallbacks

	conf *Config
	name string
}

func NewPlugin(ctx context.Context) plugintk.PluginBase {
	return plugintk.NewRegistry(NewEVMRegistry)
}

func NewEVMRegistry(callbacks plugintk.RegistryCallbacks) plugintk.RegistryAPI {
	return &evmRegistry{
		bgCtx:     context.Background(),
		callbacks: callbacks,
	}
}

func (r *evmRegistry) ConfigureRegistry(ctx context.Context, req *prototk.ConfigureRegistryRequest) (*prototk.ConfigureRegistryResponse, error) {
	r.name = req.Name

	err := json.Unmarshal([]byte(req.ConfigJson), &r.conf)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryConfig)
	}

	if r.conf.ContractAddress == nil || r.conf.ContractAddress.IsZero() {
		return nil, i18n.WrapError(ctx, err, msgs.MsgMissingContractAddress)
	}

	// Currently the configuration is static other than the contract address.
	//
	// The default behavior registry is as follows (code extension and future config enhancements will allow customization of this)
	// - Identities that contain transport addresses must be at the root of the registry contract
	// - This means the operator role of the smart contract is responsible for onboarding all nodes
	// - The node name is the name of the entry
	// - Each property of this top-level object is a transport type (such as "grpc")
	// - The value of the property is the transport details

	return &prototk.ConfigureRegistryResponse{
		RegistryConfig: &prototk.RegistryConfig{
			EventSources: []*prototk.RegistryEventSource{
				{
					ContractAddress: r.conf.ContractAddress.String(),
					AbiEventsJson:   pldtypes.JSONString(contractDetail.abi).Pretty(),
				},
			},
		},
	}, nil
}

func (r *evmRegistry) handleIdentityRegistered(ctx context.Context, inEvent *prototk.OnChainEvent) (*prototk.RegistryEntry, []*prototk.RegistryProperty, error) {
	// We should be able to parse this
	var parsedEvent IdentityRegisteredEvent
	if err := json.Unmarshal([]byte(inEvent.DataJson), &parsedEvent); err != nil {
		return nil, nil, i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryEvent, inEvent.Location)
	}

	// Check rules that the server will return errors for and we need to discard before hand
	// as the on-chain smart contract does not reject these.
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, parsedEvent.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		log.L(ctx).Warnf("Discarding %s event due to invalid entity name (%d/%d/%d): %s",
			inEvent.SoliditySignature, inEvent.Location.BlockNumber, inEvent.Location.TransactionIndex, inEvent.Location.LogIndex, err)
		// Not an error in our code
		return nil, nil, nil
	}

	entryID := parsedEvent.IdentityHash.String()

	parentID := ""
	if !parsedEvent.ParentIdentityHash.IsZero() {
		parentID = parsedEvent.ParentIdentityHash.String()
	}

	return &prototk.RegistryEntry{
			Id:       entryID,
			ParentId: parentID,
			Name:     parsedEvent.Name,
			Active:   true,
			Location: inEvent.Location,
		}, []*prototk.RegistryProperty{
			{
				EntryId:        entryID,
				Name:           "$owner", // note $ prefix for reserved name
				Value:          parsedEvent.Owner.String(),
				PluginReserved: true, // allows us to publish with $ prefix (avoiding name clash with any property name)
				Active:         true,
				Location:       inEvent.Location,
			},
		}, nil

}

func (r *evmRegistry) handlePropertySet(ctx context.Context, inEvent *prototk.OnChainEvent) (*prototk.RegistryProperty, error) {
	// We should be able to parse this
	var parsedEvent PropertySetEvent
	if err := json.Unmarshal([]byte(inEvent.DataJson), &parsedEvent); err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryEvent, inEvent.Location)
	}

	// Check rules that the server will return errors for and we need to discard before hand
	// as the on-chain smart contract does not reject these.
	if err := pldtypes.ValidateSafeCharsStartEndAlphaNum(ctx, parsedEvent.Name, pldtypes.DefaultNameMaxLen, "name"); err != nil {
		log.L(ctx).Warnf("Discarding %s event due to invalid property name (%d/%d/%d): %s",
			inEvent.SoliditySignature, inEvent.Location.BlockNumber, inEvent.Location.TransactionIndex, inEvent.Location.LogIndex, err)
		// Not an error in our code
		return nil, nil
	}

	return &prototk.RegistryProperty{
		EntryId:  parsedEvent.IdentityHash.String(),
		Name:     parsedEvent.Name,
		Value:    parsedEvent.Value,
		Active:   true,
		Location: inEvent.Location,
	}, nil
}

func (r *evmRegistry) HandleRegistryEvents(ctx context.Context, req *prototk.HandleRegistryEventsRequest) (*prototk.HandleRegistryEventsResponse, error) {

	entries := []*prototk.RegistryEntry{}
	properties := []*prototk.RegistryProperty{}

	// Parse all the events
	for _, inEvent := range req.Events {
		inSig, err := pldtypes.ParseBytes32(inEvent.Signature)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, msgs.MsgInvalidRegistryEvent, inEvent.Location)
		}
		switch {
		case contractDetail.identityRegisteredSignature.Equals(&inSig):
			regEntry, regProps, err := r.handleIdentityRegistered(ctx, inEvent)
			if err != nil {
				return nil, err
			}
			if regEntry != nil {
				entries = append(entries, regEntry)
				properties = append(properties, regProps...)
			}
		case contractDetail.propertySetSignature.Equals(&inSig):
			newProp, err := r.handlePropertySet(ctx, inEvent)
			if err != nil {
				return nil, err
			}
			if newProp != nil {
				properties = append(properties, newProp)
			}
		default:
			log.L(ctx).Infof("Discarding event unhandled by registry (%d/%d/%d): %s",
				inEvent.Location.BlockNumber, inEvent.Location.TransactionIndex, inEvent.Location.LogIndex, inEvent.SoliditySignature)
			continue
		}
	}

	return &prototk.HandleRegistryEventsResponse{
		Entries:    entries,
		Properties: properties,
	}, nil
}
