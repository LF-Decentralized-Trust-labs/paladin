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

package registrymgr

import (
	"fmt"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/toolkit/pkg/prototk"
	"github.com/stretchr/testify/require"
)

func TestGetNodeTransportsDefaultsRealDB(t *testing.T) {
	ctx, rm, tp, _, done := newTestRegistry(t, true)
	defer done()

	node1Entry := &prototk.RegistryEntry{Id: randID(), Name: "node1", Location: randChainInfo(), Active: true}
	node2Entry := &prototk.RegistryEntry{Id: randID(), Name: "node2", Location: randChainInfo(), Active: true}
	upsert1 := &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{node1Entry, node2Entry},
		Properties: []*prototk.RegistryProperty{
			newPropFor(node1Entry.Id, "organization", "Widgets 4 You"),
			newPropFor(node1Entry.Id, "transport.websockets", "things and stuff"),
			newPropFor(node2Entry.Id, "transport.websockets", "other things for other stuff"),
			newPropFor(node2Entry.Id, "transport.grpc", "proto things"),
		},
	}
	_, err := tp.r.UpsertRegistryRecords(ctx, upsert1)
	require.NoError(t, err)

	transports, err := rm.GetNodeTransports(ctx, "node1")
	require.NoError(t, err)
	require.Equal(t, []*components.RegistryNodeTransportEntry{
		{
			Node:      "node1",
			Registry:  "test1",
			Transport: "websockets",
			Details:   "things and stuff",
		},
	}, transports)

	transports, err = rm.GetNodeTransports(ctx, "node2")
	require.NoError(t, err)
	require.Len(t, transports, 2)
	require.Contains(t, transports, &components.RegistryNodeTransportEntry{
		Node:      "node2",
		Registry:  "test1",
		Transport: "websockets",
		Details:   "other things for other stuff",
	})
	require.Contains(t, transports, &components.RegistryNodeTransportEntry{
		Node:      "node2",
		Registry:  "test1",
		Transport: "grpc",
		Details:   "proto things",
	})

	_, err = rm.GetNodeTransports(ctx, "node3")
	require.Regexp(t, "PD012100", err)

	// check cache function and clear
	transports, err = rm.GetNodeTransports(ctx, "node1")
	require.NoError(t, err)
	require.Equal(t, "websockets", transports[0].Transport)

	node1Entry.Active = false
	_, err = tp.r.UpsertRegistryRecords(ctx, &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{node1Entry},
	})
	require.NoError(t, err)

	// Cache is cleared, and we no longer see it as it was inactivated
	_, err = rm.GetNodeTransports(ctx, "node1")
	require.Regexp(t, "PD012100", err)

}

func TestGetNodeTransportsCustomSettingsRealDB(t *testing.T) {
	ctx, rm, tp, _, done := newTestRegistry(t, true, func(mc *mockComponents, conf *pldconf.RegistryManagerConfig, regConf *prototk.RegistryConfig) {
		conf.Registries["test1"].Transports = pldconf.RegistryTransportsConfig{
			RequiredPrefix:    "network1.",
			HierarchySplitter: ".",
			TransportMap: map[string]string{
				"grpc": "000_grpc_a",
			},
			PropertyRegexp: "^tpt_(.*)$",
		}
	})
	defer done()

	orgAEntry := &prototk.RegistryEntry{Id: randID(), Name: "org_a", Active: true}
	orgAnode1Entry := &prototk.RegistryEntry{Id: randID(), Name: "node1", ParentId: orgAEntry.Id, Active: true}
	orgAnode2Entry := &prototk.RegistryEntry{Id: randID(), Name: "node2", ParentId: orgAEntry.Id, Active: true}
	orgBEntry := &prototk.RegistryEntry{Id: randID(), Name: "org_b", Active: true}
	orgBnode1Entry := &prototk.RegistryEntry{Id: randID(), Name: "node1", ParentId: orgBEntry.Id, Active: true}
	upsert1 := &prototk.UpsertRegistryRecordsRequest{
		Entries: []*prototk.RegistryEntry{orgAEntry, orgAnode1Entry, orgAnode2Entry, orgBEntry, orgBnode1Entry},
		Properties: []*prototk.RegistryProperty{
			newPropFor(orgAnode1Entry.Id, "tpt_grpc", "things and stuff"),
			newPropFor(orgAnode2Entry.Id, "tpt_grpc", "more things and stuff"),
			newPropFor(orgBnode1Entry.Id, "tpt_grpc", "other things and different stuff"),
		},
	}
	_, err := tp.r.UpsertRegistryRecords(ctx, upsert1)
	require.NoError(t, err)

	transports, err := rm.GetNodeTransports(ctx, "network1.org_a.node1")
	require.NoError(t, err)
	require.Equal(t, []*components.RegistryNodeTransportEntry{
		{
			Node:      "network1.org_a.node1",
			Registry:  "test1",
			Transport: "000_grpc_a",
			Details:   "things and stuff",
		},
	}, transports)

	_, err = rm.GetNodeTransports(ctx, "network2.org_a.node1")
	require.Regexp(t, "PD012100", err)

}

func TestGetNodeTransportsErr(t *testing.T) {
	ctx, rm, _, m, done := newTestRegistry(t, false)
	defer done()

	m.db.ExpectQuery("SELECT.*reg_entries").WillReturnError(fmt.Errorf("pop"))

	_, err := rm.GetNodeTransports(ctx, "node1")
	require.Regexp(t, "pop", err)
}

func TestBadTransportLookupPropertyRegexp(t *testing.T) {
	_, rm, mc, done := newTestRegistryManager(t, false, &pldconf.RegistryManagerConfig{
		Registries: map[string]*pldconf.RegistryConfig{
			"test1": {
				Transports: pldconf.RegistryTransportsConfig{
					Enabled: confutil.P(false),
				},
				Config: map[string]any{"some": "conf"},
			},
			"test2": {
				Transports: pldconf.RegistryTransportsConfig{
					PropertyRegexp: "(((((!!! wrong",
				},
				Config: map[string]any{"some": "conf"},
			},
		},
	}, func(mc *mockComponents) { mc.noInit = true })
	defer done()

	_, err := rm.PreInit(mc.allComponents)
	require.Regexp(t, "PD012108.*test2", err)

}
