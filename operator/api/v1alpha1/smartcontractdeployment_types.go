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

// SmartContractDeploymentSpec defines the desired state of SmartContractDeployment
type SmartContractDeploymentSpec struct {
	// This CR will wait for the deployment of all contracts in this list, before
	// parsing the bytecode for deployment. This allows unlinked dependencies
	// to be linked before deployment.
	RequiredContractDeployments []string `json:"requiredContractDeployments,omitempty"`

	// The node to use to deploy - reference to a PaladinNode CR
	Node string `json:"node"`
	// +kubebuilder:validation:Enum=public;private
	// +kubebuilder:default=public
	// Type of transaction to submit to Paladin
	TxType string `json:"txType"`
	// Domain for private transactions
	Domain string `json:"domain,omitempty"`
	// The ABI of the smart contract - provides the constructor parameter definition
	ABIJSON string `json:"abiJSON"`
	// The bytecode of the smart contract
	Bytecode string `json:"bytecode"`
	// Reference to the signing key to use to deploy
	From string `json:"from"`
	// JSON parameter data (array, object, or empty if no params)
	ParamsJSON string `json:"paramsJSON,omitempty"`

	// Unlinked contracts have list of the references that need to be resolve, alongside the bytecode
	LinkReferencesJSON string `json:"linkReferencesJSON,omitempty"`
	// If the bytecode is unlinked, then this map will be used to resolve the dependencies.
	// The keys in the map are the library name, which can be optionally fully qualified
	// with the syntax FileName.sol:LibName. An entry must be provided for every
	// unlinked dependency, or the CR will not perform a deployment.
	//
	// The values are evaluated as go templates, with access to the CR.
	// So you can refer to .status.resolvedContractAddresses in the values via go templating.
	// See https://docs.soliditylang.org/en/latest/using-the-compiler.html#library-linking for detail
	LinkedContracts map[string]string `json:"linkedContracts,omitempty"`
}

type TransactionStatus string

const (
	TransactionStatusSubmitting TransactionStatus = "Submitting"
	TransactionStatusPending    TransactionStatus = "Pending"
	TransactionStatusSuccess    TransactionStatus = "Success"
	TransactionStatusFailed     TransactionStatus = "Failed"
	TransactionStatusRejected   TransactionStatus = "Rejected"
)

type TransactionSubmission struct {
	TransactionStatus TransactionStatus `json:"transactionStatus,omitempty"`
	IdempotencyKey    string            `json:"idempotencyKey,omitempty"`
	TransactionID     string            `json:"transactionID,omitempty"`
	FailureMessage    string            `json:"failureMessage,omitempty"`
	TransactionHash   string            `json:"transactionHash,omitempty"`
}

type ContactDependenciesStatus struct {
	ContractDepsSummary       string            `json:"contractDepsSummary,omitempty"`
	ResolvedContractAddresses map[string]string `json:"resolvedContractAddresses,omitempty"`
}

// SmartContractDeploymentStatus defines the observed state of SmartContractDeployment
type SmartContractDeploymentStatus struct {
	ContactDependenciesStatus `json:",inline"`
	TransactionSubmission     `json:",inline"`
	ContractAddress           string `json:"contractAddress,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName="scd"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.transactionStatus`
//+kubebuilder:printcolumn:name="Deps",type="string",JSONPath=`.status.contractDepsSummary`
//+kubebuilder:printcolumn:name="TransactionID",type="string",JSONPath=`.status.transactionID`
//+kubebuilder:printcolumn:name="Contract",type="string",JSONPath=`.status.contractAddress`
//+kubebuilder:printcolumn:name="TxHash",type="string",JSONPath=`.status.transactionHash`
//+kubebuilder:printcolumn:name="Failure",type="string",JSONPath=`.status.failureMessage`

// SmartContractDeployment is the Schema for the smartcontractdeployments API
type SmartContractDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmartContractDeploymentSpec   `json:"spec,omitempty"`
	Status SmartContractDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SmartContractDeploymentList contains a list of SmartContractDeployment
type SmartContractDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartContractDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SmartContractDeployment{}, &SmartContractDeploymentList{})
}
