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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BesuSpec defines the desired state of Besu
type BesuSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Settings from this config will be loaded as TOML and used as the base of the configuration.
	Config *string `json:"config,omitempty"`
	// The name of the genesis CR that these nodes will use to obtain their genesis file, and find bootnodes
	Genesis     string                           `json:"genesis"`
	PVCTemplate corev1.PersistentVolumeClaimSpec `json:"pvcTemplate,omitempty"`

	// Optionally tune the service definition.
	// We merge any configuration you add (such as node ports) for the following services:
	// "rpc-http" - 8545 (TCP),
	// "rpc-ws" - 8546 (TCP),
	// "graphql-http" - 8547 (TCP),
	// "p2p-tcp" - 30303 (TCP),
	// "p2p-udp" - 30303 (UDP)
	Service corev1.ServiceSpec `json:"service,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=`.status.phase`

// Besu is the Schema for the besus API
type Besu struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BesuSpec `json:"spec,omitempty"`
	Status Status   `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BesuList contains a list of Besu
type BesuList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Besu `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Besu{}, &BesuList{})
}
