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
package pldconf

import (
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
)

type RegistryManagerConfig struct {
	Registries      map[string]*RegistryConfig   `json:"registries"`
	RegistryManager RegistryManagerManagerConfig `json:"registryManager"`
}

type RegistryManagerManagerConfig struct {
	RegistryCache CacheConfig `json:"registryCache"`
}

var RegistryCacheDefaults = &CacheConfig{
	Capacity: confutil.P(100),
}

type RegistryInitConfig struct {
	Retry RetryConfig `json:"retry"`
}

type RegistryConfig struct {
	Init       RegistryInitConfig       `json:"init"`
	Transports RegistryTransportsConfig `json:"transports"`
	Plugin     PluginConfig             `json:"plugin"`
	Config     map[string]any           `json:"config"`
}

type RegistryTransportsConfig struct {

	// If true, then this registry will be used for lookup of node transports
	Enabled *bool

	// Prefix if set that will be matched and cut from any supplied lookup
	// node name before performing a lookup. If it does not match (or matches
	// the whole lookup) then this registry will not be used to lookup the node.
	// This allows multiple registries to be used safely for different
	// private node connectivity networks without any possibility
	// of clashing node names.
	RequiredPrefix string `json:"requiredPrefix"`

	// By default the whole node name must match a root entry in the registry.
	// If a hierarchySplitter is provided (such as ".") then the supplied node
	// name will be split into path parts and each entry in the hierarchy
	// will be resolved in order, from the root down to the leaf.
	HierarchySplitter string `json:"hierarchySplitter"`

	// If a node is found, then each property name will be applied to this
	// regular expression, and if it matches then the value of the property
	// will be considered a set of transport details.
	//
	// The transport name must be extracted as a match group.
	//
	// For example the default is:
	//   propertyRegexp: "^transport.(.*)$"
	//
	// This will match a property called "transport.grpc" as the transport
	// details for the grpc transport.
	PropertyRegexp string `json:"propertyRegexp"`

	// Optionally add entries here to map from the name of a transport as stored in
	// the registry, to the name in your local configuration.
	// This allows you to use different configurations (MTLS certs etc.)
	// for different private node networks that all use the same logical
	// transport name.
	TransportMap map[string]string
}

var RegistryTransportsDefaults = &RegistryTransportsConfig{
	Enabled:        confutil.P(true),
	PropertyRegexp: "^transport.(.*)$",
}
