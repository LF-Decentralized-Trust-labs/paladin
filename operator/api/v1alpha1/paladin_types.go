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

// PaladinSpec defines the desired state of Paladin
type PaladinSpec struct {
	// Settings from this config will be loaded as YAML and used as the base of the configuration.
	Config *string `json:"config,omitempty"`

	// Database section k8s native functions for setting up the database
	// with auto-generation/auto-edit of the DB related config sections
	Database Database `json:"database,omitempty"`

	// Adds signing modules that load their key materials from a k8s secret
	SecretBackedSigners []SecretBackedSigner `json:"secretBackedSigners,omitempty"`

	// Optionally bind to a local besu node deployed with this operator
	// (vs. configuring a connection to a production blockchain network)
	BesuNode string `json:"besuNode,omitempty"`

	// Optionally tune the service definition.
	// We merge any configuration you add (such as node ports) for the following services:
	// "rpc-http" - 8545 (TCP),
	// "rpc-ws" - 8546 (TCP)
	Service corev1.ServiceSpec `json:"service,omitempty"`

	// A list of domains to merge into the configuration, and rebuild the config of paladin when this list changes
	Domains []DomainReference `json:"domains"`

	// A list of registries to merge into the configuration, and rebuild the config of paladin when this list changes
	Registries []RegistryReference `json:"registries"`

	// Transports are configured individually on each node, as they reference security details specific to that node
	Transports []TransportConfig `json:"transports"`
}

type LabelReference struct {
	// Label selectors provide a flexible many-to-many mapping between nodes and domains in a namespace.
	// The domain CRs you reference must be labelled to match. For example you could use a label like "paladin.io/domain-name" to select by name.
	LabelSelector metav1.LabelSelector `json:"labelSelector"`
}

type TransportConfig struct {
	Name string `json:"name"`
	// Plugin configuration for loading the transport
	Plugin PluginConfig `json:"plugin"`
	// JSON configuration specific to the individual transport.
	// The "tls" root section of the config will be automatically populated from the k8s secret.
	// The "externalHostname" property will be automatically set to the internal k8s hostname, unless it is already set in the configJSON
	ConfigJSON string `json:"configJSON"`
	// TLS configuration to use for this secret
	TLS TLSConfig `json:"tls,omitempty"`
	// The port number to listen on this transport
	Ports []corev1.ServicePort `json:"ports"`
}

type TLSConfig struct {
	// Secret name is required
	SecretName string `json:"secretName"`
	// If specified then a cert-manager.io/v1 Certificate will be created for the internal DNS names of the service.
	// If you define multiple transports that share a secret, then only specify this on one.
	CertName string `json:"certName"`
	// Issuer for the certificate if a certificateName is specified (note cluster issuer can be used with a custom certSpecTemplate)
	// +kubebuilder:default=selfsigned-issuer
	Issuer string `json:"issuer,omitempty"`
	// Additional DNS names to add to the definition (for external hostnames) when using automatic cert-manager
	AdditionalDNSNames []string `json:"additionalDNSNames,omitempty"`
	// Go template for the YAML spec of the issuer CR, which will have access to the inserts when building:
	// {{.nodeName}} {{.dnsNames}} {{.secretName}} {{.issuer}}
	// Where .nodeName is that placed in the config
	// This approach allows us to avoid a build-time dependency on the CertManager CRs, while letting you
	// customize things like the algorithm.
	CertSpecTemplate string `json:"certSpecTemplate,omitempty"`
}

// Each domain reference can select one or more domains to include via label selectors
// Most common to use a simple one-reference-per-domain approach.
type DomainReference struct {
	LabelReference `json:",inline"`
}

// Each registry reference can select one or more domains to include via label selectors
// Most common to use a simple one-reference-per-domain approach.
type RegistryReference struct {
	LabelReference `json:",inline"`
}

const DBMode_EmbeddedSQLite = "embeddedSQLite"
const DBMode_SidecarPostgres = "sidecarPostgres"
const DBMigrationMode_Auto = "auto"

// Database configuration
type Database struct {
	// +kubebuilder:validation:Enum=preConfigured;sidecarPostgres;embeddedSQLite
	// +kubebuilder:default=preConfigured
	Mode string `json:"mode,omitempty"`
	// +kubebuilder:validation:Enum=preConfigured;auto
	// +kubebuilder:default=preConfigured
	MigrationMode string `json:"migrationMode,omitempty"`
	// If set then {{.username}} and {{.password}} variables will be available in your DSN
	PasswordSecret *string                          `json:"passwordSecret,omitempty"`
	PVCTemplate    corev1.PersistentVolumeClaimSpec `json:"pvcTemplate,omitempty"`
}

const SignerType_AutoHDWallet = "autoHDWallet"

type SecretBackedSigner struct {
	Secret string `json:"secret"`
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	Name string `json:"name"` // TODO: Currently only one signer supported in Paladin until key manager in place
	// +kubebuilder:validation:Enum=autoHDWallet;preConfigured
	// +kubebuilder:default=autoHDWallet
	// The operator supports generating the seed and base config for a simple seeded BIP32 HDWallet signer.
	// If more other options are needed, these can be set directly in the YAML config for this signer.
	Type string `json:"type"`
	// Wallets will be evaluated against new allocations of key identifiers in the order they are
	// defined. The key selector regular expression allows wallets to sub-select, with more specific
	// rules first on key matching and more generic rules (like the default of ".*") last.
	// +kubebuilder:default=.*
	KeySelector string `json:"keySelector"`
}

// StatusReason is an enumeration of possible failure causes.  Each StatusReason
// must map to a single HTTP status code, but multiple reasons may map
// to the same HTTP status code.
// TODO: move to apiserver
type StatusReason string

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=`.status.phase`

// Paladin is the Schema for the paladin API
type Paladin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PaladinSpec `json:"spec,omitempty"`
	Status Status      `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PaladinList contains a list of Paladin
type PaladinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Paladin `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Paladin{}, &PaladinList{})
}
