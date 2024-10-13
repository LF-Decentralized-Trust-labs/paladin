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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BesuGenesisSpec defines the desired state of BesuGenesis
// All Besu nodes must be bound to a genesis, and will attempt to peer with any other nodes in the same namespace using the same genesis.
type BesuGenesisSpec struct {
	// Base JSON genesis file will be loaded in and then modified as appropriate.
	// Note only modelled fields of the genesis are supported, so check besugenesis.GenesisJSON for support of the field you wish to modify
	Base string `json:"base,omitempty"`
	// The chain ID - must not change after creation without chain reset
	ChainID uint64 `json:"chainID"`
	// The initial gas limit - must not change after creation without chain reset (node config be used to increase gas limit incrementally in new blocks)
	GasLimit uint64 `json:"gasLimit"`
	// +kubebuilder:validation:Enum=qbft
	Consensus string `json:"consensus"`
	// Block period can be in seconds (s) or milliseconds - cannot be changed once set (used in genesis generation)
	BlockPeriod string `json:"blockPeriod"`
	// EmptyBlockPeriod period will be rounded to seconds regardless of units used
	EmptyBlockPeriod string `json:"emptyBlockPeriod,omitempty"`
	// List of nodes that are included in the genesis block validators.
	// The CRs for these must created for the genesis to form, as it requires the identity secrets of those nodes.
	// Cannot be changed once set (used in genesis generation).
	InitialValidators []string `json:"initialValidators"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BesuGenesis is the Schema for the besugeneses API
type BesuGenesis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BesuGenesisSpec `json:"spec,omitempty"`
	Status Status          `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BesuGenesisList contains a list of BesuGenesis
type BesuGenesisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BesuGenesis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BesuGenesis{}, &BesuGenesisList{})
}
