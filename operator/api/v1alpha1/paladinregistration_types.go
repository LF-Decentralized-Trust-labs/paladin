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

// PaladinRegistrationSpec defines the desired state of PaladinRegistration
type PaladinRegistrationSpec struct {
	// Reference to the Registry CR - must be of type "evm" for the registration to process
	Registry string `json:"registry"`
	// The node to use to submit the registration with access to the admin key
	RegistryAdminNode string `json:"registryAdminNode"`
	// The key to use to sign the transactions
	RegistryAdminKey string `json:"registryAdminKey"`
	// The node to publish the registration for - owns its registration key
	Node string `json:"node"`
	// The key to use on the node to publish its endpoint information
	NodeKey string `json:"nodeAdminKey"`
	// The transports to publish - we'll wait for them to become available, in the order specified here
	Transports []string `json:"transports,omitempty"`
}

// PaladinRegistrationStatus defines the observed state of PaladinRegistration
type PaladinRegistrationStatus struct {
	PublishCount   int                              `json:"publishCount"`
	RegistrationTx TransactionSubmission            `json:"registrationTx"`
	PublishTxs     map[string]TransactionSubmission `json:"publishTxs"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:resource:shortName="reg"
//+kubebuilder:printcolumn:name="Published",type="string",JSONPath=`.status.publishCount`

// PaladinRegistration is the Schema for the paladinregistrations API
type PaladinRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PaladinRegistrationSpec   `json:"spec,omitempty"`
	Status PaladinRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PaladinRegistrationList contains a list of PaladinRegistration
type PaladinRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PaladinRegistration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PaladinRegistration{}, &PaladinRegistrationList{})
}
