/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RegistryType string

const (
	RegistryTypeEVM RegistryType = "evm"
)

// PaladinRegistrySpec defines the desired state of PaladinRegistry
type PaladinRegistrySpec struct {
	// +kubebuilder:validation:Enum=evm
	// +kubebuilder:default=evm
	Type RegistryType `json:"type"`
	// Config specific to EVM based registry
	EVM EVMRegistryConfig `json:"evm,omitempty"`
	// Optionally adjust how the transport configuration works
	Transports RegistryTransportsConfig `json:"transports,omitempty"`
	// Details of the plugin to load for the domain
	Plugin PluginConfig `json:"plugin"`
	// JSON configuration specific to the individual registry
	ConfigJSON string `json:"configJSON"`
}

type EVMRegistryConfig struct {
	// Reference to a SmartContractDeployment CR that is used to deploy the registry
	SmartContractDeployment string `json:"smartContractDeployment,omitempty"`
	// If you have separately deployed the registry, supply the registry address directly
	ContractAddress string `json:"contractAddress,omitempty"`
}

type RegistryTransportsConfig struct {
	// If true, then this registry will be used for lookup of node transports
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Prefix if set that will be matched and cut from any supplied lookup
	// node name before performing a lookup. If it does not match (or matches
	// the whole lookup) then this registry will not be used to lookup the node.
	// This allows multiple registries to be used safely for different
	// private node connectivity networks without any possibility
	// of clashing node names.
	RequiredPrefix string `json:"requiredPrefix,omitempty"`

	// By default the whole node name must match a root entry in the registry.
	// If a hierarchySplitter is provided (such as ".") then the supplied node
	// name will be split into path parts and each entry in the hierarchy
	// will be resolved in order, from the root down to the leaf.
	HierarchySplitter string `json:"hierarchySplitter,omitempty"`

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
	PropertyRegexp string `json:"propertyRegexp,omitempty"`

	// Optionally add entries here to map from the name of a transport as stored in
	// the registry, to the name in your local configuration.
	// This allows you to use different configurations (MTLS certs etc.)
	// for different private node networks that all use the same logical
	// transport name.
	TransportMap map[string]string `json:"transportMap,omitempty"`
}

type RegistryStatus string

const (
	RegistryStatusPending   RegistryStatus = "Pending"
	RegistryStatusAvailable RegistryStatus = "Available"
)

// PaladinRegistryStatus defines the observed state of PaladinRegistry
type PaladinRegistryStatus struct {
	Status          RegistryStatus `json:"status"`
	ContractAddress string         `json:"contractAddress,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:shortName="registry"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=`.spec.type`
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Contract",type="string",JSONPath=`.status.contractAddress`

// PaladinRegistry is the Schema for the paladinregistries API
type PaladinRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PaladinRegistrySpec   `json:"spec,omitempty"`
	Status PaladinRegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PaladinRegistryList contains a list of PaladinRegistry
type PaladinRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PaladinRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PaladinRegistry{}, &PaladinRegistryList{})
}
