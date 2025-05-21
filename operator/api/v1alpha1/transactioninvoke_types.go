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

// TransactionInvokeSpec defines the desired state of TransactionInvoke
type TransactionInvokeSpec struct {
	// A list of pre-requisite smart contract deployments that must be resolved
	// to contract addresses before the transaction can be built+submitted.
	// The set of smart contracts is built one-by-one as the smart contract deployments
	// complete, and once set a dependency does not change if the deployment CR
	// is deleted and re-created. So it is important to delete+recreate all
	// inter-related SmartContractDeployment and TransactionInvoke CRs in a set
	// when they are being used as a deployment engine for test infrastructure.
	//
	// This is not intended as substitute to proper smart contract management
	// in production. Instead it is an excellent tool for rapid re-deployment
	// of test infrastructure.
	ContractDeploymentDeps []string `json:"requiredContractDeployments,omitempty"`

	// The node to use to deploy - reference to a PaladinNode CR
	Node string `json:"node"`
	// +kubebuilder:validation:Enum=public;private
	// +kubebuilder:default=public
	// Type of transaction to submit to Paladin
	TxType string `json:"txType"`
	// Domain for private transactions
	Domain string `json:"domain,omitempty"`
	// The name or full signature of the function to invoke
	Function string `json:"function"`
	// The ABI of the smart contract - provides the constructor parameter definition
	ABIJSON string `json:"abiJSON"`
	// Reference to the signing key to use to deploy
	From string `json:"from"`
	// Go template that specifies the target smart contract for invocation.
	// See paramsJSONTemplate for more detail
	ToTemplate string `json:"toTemplate,omitempty"`
	// Go template that specifies the data JSON payload for the invocation
	// of the smart contract (array of input values, or map of inputs by name).
	// Once all pre-requisite contract deployments are completed, this template
	// will be executed with the JSON serialization of CR as the input
	// to the CR execution. As such it has access to fields like:
	// .status.resolvedContractAddresses
	ParamsJSONTemplate string `json:"paramsJSONTemplate,omitempty"`
}

// TransactionInvokeStatus defines the observed state of TransactionInvoke
type TransactionInvokeStatus struct {
	ContactDependenciesStatus `json:",inline"`
	TransactionSubmission     `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:shortName="txn"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.transactionStatus`
//+kubebuilder:printcolumn:name="Deps",type="string",JSONPath=`.status.contractDepsSummary`
//+kubebuilder:printcolumn:name="TransactionID",type="string",JSONPath=`.status.transactionID`
//+kubebuilder:printcolumn:name="TxHash",type="string",JSONPath=`.status.transactionHash`
//+kubebuilder:printcolumn:name="Failure",type="string",JSONPath=`.status.failureMessage`

// TransactionInvoke is the Schema for the transactioninvokes API
type TransactionInvoke struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TransactionInvokeSpec   `json:"spec,omitempty"`
	Status TransactionInvokeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TransactionInvokeList contains a list of TransactionInvoke
type TransactionInvokeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TransactionInvoke `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TransactionInvoke{}, &TransactionInvokeList{})
}
