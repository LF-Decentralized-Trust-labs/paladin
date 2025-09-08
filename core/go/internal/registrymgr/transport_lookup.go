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
	"context"
	"regexp"
	"strings"

	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/i18n"
	"github.com/LF-Decentralized-Trust-labs/paladin/common/go/pkg/log"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/components"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/internal/msgs"
	"github.com/LF-Decentralized-Trust-labs/paladin/core/pkg/persistence"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
)

type transportLookup struct {
	regName           string
	requiredPrefix    string
	hierarchySplitter string
	transportNameMap  map[string]string
	propertyRegexp    *regexp.Regexp
}

func newTransportLookup(ctx context.Context, regName string, conf *pldconf.RegistryTransportsConfig) (tl *transportLookup, err error) {
	tl = &transportLookup{
		regName:           regName,
		requiredPrefix:    confutil.StringNotEmpty(&conf.RequiredPrefix, pldconf.RegistryTransportsDefaults.RequiredPrefix),
		hierarchySplitter: confutil.StringNotEmpty(&conf.HierarchySplitter, pldconf.RegistryTransportsDefaults.HierarchySplitter),
		transportNameMap:  map[string]string{},
	}

	tl.propertyRegexp, err = regexp.Compile(
		confutil.StringNotEmpty(&conf.PropertyRegexp, pldconf.RegistryTransportsDefaults.PropertyRegexp),
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, msgs.MsgRegistryTransportPropertyRegexp, regName)
	}

	for k, v := range conf.TransportMap {
		tl.transportNameMap[k] = v
	}

	return tl, nil
}

func (tl *transportLookup) getNodeTransports(ctx context.Context, dbTX persistence.DBTX, r *registry, fullLookup string) ([]*components.RegistryNodeTransportEntry, error) {

	lookup := fullLookup
	if tl.requiredPrefix != "" {
		noPrefix, matched := strings.CutPrefix(fullLookup, tl.requiredPrefix)
		if !matched {
			log.L(ctx).Infof("Node lookup '%s' did not match required prefix for registry '%s' (requiredPrefix='%s')",
				fullLookup, tl.regName, tl.requiredPrefix)
			return nil, nil
		}
		lookup = noPrefix
	}

	hierarchy := []string{lookup}
	if tl.hierarchySplitter != "" {
		hierarchy = strings.Split(lookup, tl.hierarchySplitter)
	}

	// Resolve all the items in the hierarchy to find the leaf
	var lookupParentID pldtypes.HexBytes
	var entry *pldapi.RegistryEntryWithProperties
	for _, entryName := range hierarchy {
		q := query.NewQueryBuilder().Equal(".name", entryName).Limit(1)
		if lookupParentID == nil {
			q = q.Null(".parentId")
		} else {
			q = q.Equal(".parentId", lookupParentID)
		}
		entries, err := r.QueryEntriesWithProps(ctx, dbTX, pldapi.ActiveFilterActive, q.Query())
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			log.L(ctx).Infof("Node lookup '%s' did not match an entry in registry '%s' (fullLookup='%s',requiredPrefix='%s',parentId='%s')",
				entryName, tl.regName, lookup, tl.requiredPrefix, lookupParentID)
			return nil, nil
		}
		entry = entries[0]
		lookupParentID = entry.ID
	}

	// We now have a node that we trust with a matching name, go through the properties to find matching transports.
	log.L(ctx).Infof("Node lookup '%s' matched to entry ID '%s' in registry '%s'", fullLookup, entry.ID, tl.regName)
	var transports []*components.RegistryNodeTransportEntry
	for k, v := range entry.Properties {
		subMatch := tl.propertyRegexp.FindStringSubmatch(k)
		if len(subMatch) != 2 {
			log.L(ctx).Debugf("Property '%s' does not match regexp '%s'", k, tl.propertyRegexp)
			continue
		}
		transportName := subMatch[1]
		mappedName := tl.transportNameMap[transportName]
		if mappedName != "" {
			transportName = mappedName
		}
		log.L(ctx).Infof("Property '%s' matches transport %s (mappedName=%s,regexp='%s')", k, subMatch[1], transportName, tl.propertyRegexp)
		transports = append(transports, &components.RegistryNodeTransportEntry{
			Node:      fullLookup,
			Registry:  tl.regName,
			Transport: transportName,
			Details:   v,
		})
	}
	return transports, nil
}
