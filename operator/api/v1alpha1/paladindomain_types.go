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

// PaladinDomainSpec defines the desired state of PaladinDomain
type PaladinDomainSpec struct {
	// Reference to a SmartContractDeployment CR that is used to deploy a new registry contract
	SmartContractDeployment string `json:"smartContractDeployment,omitempty"`
	// If you have separately deployed the registry for this domain, supply the registry address directly
	RegistryAddress string `json:"registryAddress,omitempty"`
	// Details of the plugin to load for the domain
	Plugin PluginConfig `json:"plugin"`
	// Whether the code inside of this domain is allowed to perform processing using in-memory key materials.
	// Required when Zero Knowledge Proof (ZKP) generation is being co-located with the Paladin core process
	// for domains like Zeto.
	AllowSigning bool `json:"allowSigning,omitempty"`
	// JSON configuration specific to the individual domain
	ConfigJSON string `json:"configJSON"`
}

type PluginConfig struct {
	// +kubebuilder:validation:Enum=c-shared;jar
	// The library type to load
	Type string `json:"type"`
	// The location of the library - do not include the "lib" prefix or the ".so" suffix for shared libraries
	Library string `json:"library"`
	// For Java only, the name of the class to load from the Jar
	Class *string `json:"class,omitempty"`
}

type DomainStatus string

const (
	DomainStatusPending   DomainStatus = "Pending"
	DomainStatusAvailable DomainStatus = "Available"
)

// PaladinDomainStatus defines the observed state of PaladinDomain
type PaladinDomainStatus struct {
	Status          DomainStatus `json:"status"`
	RegistryAddress string       `json:"registryAddress,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName="domain"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.status`
// +kubebuilder:printcolumn:name="Domain_Registry",type="string",JSONPath=`.status.registryAddress`
// +kubebuilder:printcolumn:name="Deployment",type="string",JSONPath=`.spec.smartContractDeployment`
// +kubebuilder:printcolumn:name="Library",type="string",JSONPath=`.spec.plugin.library`

// PaladinDomain is the Schema for the paladindomains API
type PaladinDomain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PaladinDomainSpec   `json:"spec,omitempty"`
	Status PaladinDomainStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PaladinDomainList contains a list of PaladinDomain
type PaladinDomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PaladinDomain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PaladinDomain{}, &PaladinDomainList{})
}
