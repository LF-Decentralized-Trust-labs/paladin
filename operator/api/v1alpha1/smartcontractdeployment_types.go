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
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Enum=public;private
	// +kubebuilder:default=public
	// Type of transaction to submit to Paladin
	TxType string `json:"txType"`
	// The ABI of the smart contract - provides the constructor parameter definition
	ABI string `json:"abi"`
	// The bytecode of the smart contract
	Bytecode string `json:"bytecode"`
	// The node to use to deploy - reference to a PaladinNode CR
	DeployNode string `json:"deployNode"`
	// Reference to the signing key to use to deploy
	DeployKey string `json:"deployKey"`
	// Domain for private transactions
	Domain string `json:"domain,omitempty"`
	// JSON parameter data (array, object, or empty if no params)
	ParamsJSON string `json:"paramsJSON,omitempty"`
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

// SmartContractDeploymentStatus defines the observed state of SmartContractDeployment
type SmartContractDeploymentStatus struct {
	TransactionSubmission `json:",inline"`
	ContractAddress       string `json:"contractAddress,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName="scd"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=`.status.transactionStatus`
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
