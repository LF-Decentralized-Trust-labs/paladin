//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Besu) DeepCopyInto(out *Besu) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Besu.
func (in *Besu) DeepCopy() *Besu {
	if in == nil {
		return nil
	}
	out := new(Besu)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Besu) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BesuGenesis) DeepCopyInto(out *BesuGenesis) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BesuGenesis.
func (in *BesuGenesis) DeepCopy() *BesuGenesis {
	if in == nil {
		return nil
	}
	out := new(BesuGenesis)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BesuGenesis) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BesuGenesisList) DeepCopyInto(out *BesuGenesisList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BesuGenesis, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BesuGenesisList.
func (in *BesuGenesisList) DeepCopy() *BesuGenesisList {
	if in == nil {
		return nil
	}
	out := new(BesuGenesisList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BesuGenesisList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BesuGenesisSpec) DeepCopyInto(out *BesuGenesisSpec) {
	*out = *in
	if in.InitialValidators != nil {
		in, out := &in.InitialValidators, &out.InitialValidators
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BesuGenesisSpec.
func (in *BesuGenesisSpec) DeepCopy() *BesuGenesisSpec {
	if in == nil {
		return nil
	}
	out := new(BesuGenesisSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BesuList) DeepCopyInto(out *BesuList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Besu, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BesuList.
func (in *BesuList) DeepCopy() *BesuList {
	if in == nil {
		return nil
	}
	out := new(BesuList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BesuList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BesuSpec) DeepCopyInto(out *BesuSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = new(string)
		**out = **in
	}
	in.PVCTemplate.DeepCopyInto(&out.PVCTemplate)
	in.Service.DeepCopyInto(&out.Service)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BesuSpec.
func (in *BesuSpec) DeepCopy() *BesuSpec {
	if in == nil {
		return nil
	}
	out := new(BesuSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Database) DeepCopyInto(out *Database) {
	*out = *in
	if in.PasswordSecret != nil {
		in, out := &in.PasswordSecret, &out.PasswordSecret
		*out = new(string)
		**out = **in
	}
	in.PVCTemplate.DeepCopyInto(&out.PVCTemplate)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Database.
func (in *Database) DeepCopy() *Database {
	if in == nil {
		return nil
	}
	out := new(Database)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DomainReference) DeepCopyInto(out *DomainReference) {
	*out = *in
	in.LabelReference.DeepCopyInto(&out.LabelReference)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DomainReference.
func (in *DomainReference) DeepCopy() *DomainReference {
	if in == nil {
		return nil
	}
	out := new(DomainReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EVMRegistryConfig) DeepCopyInto(out *EVMRegistryConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EVMRegistryConfig.
func (in *EVMRegistryConfig) DeepCopy() *EVMRegistryConfig {
	if in == nil {
		return nil
	}
	out := new(EVMRegistryConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LabelReference) DeepCopyInto(out *LabelReference) {
	*out = *in
	in.LabelSelector.DeepCopyInto(&out.LabelSelector)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LabelReference.
func (in *LabelReference) DeepCopy() *LabelReference {
	if in == nil {
		return nil
	}
	out := new(LabelReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Paladin) DeepCopyInto(out *Paladin) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Paladin.
func (in *Paladin) DeepCopy() *Paladin {
	if in == nil {
		return nil
	}
	out := new(Paladin)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Paladin) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinDomain) DeepCopyInto(out *PaladinDomain) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinDomain.
func (in *PaladinDomain) DeepCopy() *PaladinDomain {
	if in == nil {
		return nil
	}
	out := new(PaladinDomain)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinDomain) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinDomainList) DeepCopyInto(out *PaladinDomainList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PaladinDomain, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinDomainList.
func (in *PaladinDomainList) DeepCopy() *PaladinDomainList {
	if in == nil {
		return nil
	}
	out := new(PaladinDomainList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinDomainList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinDomainSpec) DeepCopyInto(out *PaladinDomainSpec) {
	*out = *in
	in.Plugin.DeepCopyInto(&out.Plugin)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinDomainSpec.
func (in *PaladinDomainSpec) DeepCopy() *PaladinDomainSpec {
	if in == nil {
		return nil
	}
	out := new(PaladinDomainSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinDomainStatus) DeepCopyInto(out *PaladinDomainStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinDomainStatus.
func (in *PaladinDomainStatus) DeepCopy() *PaladinDomainStatus {
	if in == nil {
		return nil
	}
	out := new(PaladinDomainStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinList) DeepCopyInto(out *PaladinList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Paladin, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinList.
func (in *PaladinList) DeepCopy() *PaladinList {
	if in == nil {
		return nil
	}
	out := new(PaladinList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistration) DeepCopyInto(out *PaladinRegistration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistration.
func (in *PaladinRegistration) DeepCopy() *PaladinRegistration {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinRegistration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistrationList) DeepCopyInto(out *PaladinRegistrationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PaladinRegistration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistrationList.
func (in *PaladinRegistrationList) DeepCopy() *PaladinRegistrationList {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistrationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinRegistrationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistrationSpec) DeepCopyInto(out *PaladinRegistrationSpec) {
	*out = *in
	if in.Transports != nil {
		in, out := &in.Transports, &out.Transports
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistrationSpec.
func (in *PaladinRegistrationSpec) DeepCopy() *PaladinRegistrationSpec {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistrationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistrationStatus) DeepCopyInto(out *PaladinRegistrationStatus) {
	*out = *in
	out.RegistrationTx = in.RegistrationTx
	if in.PublishTxs != nil {
		in, out := &in.PublishTxs, &out.PublishTxs
		*out = make(map[string]TransactionSubmission, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistrationStatus.
func (in *PaladinRegistrationStatus) DeepCopy() *PaladinRegistrationStatus {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistrationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistry) DeepCopyInto(out *PaladinRegistry) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistry.
func (in *PaladinRegistry) DeepCopy() *PaladinRegistry {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistry)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinRegistry) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistryList) DeepCopyInto(out *PaladinRegistryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PaladinRegistry, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistryList.
func (in *PaladinRegistryList) DeepCopy() *PaladinRegistryList {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistryList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PaladinRegistryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistrySpec) DeepCopyInto(out *PaladinRegistrySpec) {
	*out = *in
	out.EVM = in.EVM
	in.Transports.DeepCopyInto(&out.Transports)
	in.Plugin.DeepCopyInto(&out.Plugin)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistrySpec.
func (in *PaladinRegistrySpec) DeepCopy() *PaladinRegistrySpec {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistrySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinRegistryStatus) DeepCopyInto(out *PaladinRegistryStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinRegistryStatus.
func (in *PaladinRegistryStatus) DeepCopy() *PaladinRegistryStatus {
	if in == nil {
		return nil
	}
	out := new(PaladinRegistryStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PaladinSpec) DeepCopyInto(out *PaladinSpec) {
	*out = *in
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = new(string)
		**out = **in
	}
	in.Database.DeepCopyInto(&out.Database)
	if in.SecretBackedSigners != nil {
		in, out := &in.SecretBackedSigners, &out.SecretBackedSigners
		*out = make([]SecretBackedSigner, len(*in))
		copy(*out, *in)
	}
	in.Service.DeepCopyInto(&out.Service)
	if in.Domains != nil {
		in, out := &in.Domains, &out.Domains
		*out = make([]DomainReference, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Registries != nil {
		in, out := &in.Registries, &out.Registries
		*out = make([]RegistryReference, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Transports != nil {
		in, out := &in.Transports, &out.Transports
		*out = make([]TransportConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PaladinSpec.
func (in *PaladinSpec) DeepCopy() *PaladinSpec {
	if in == nil {
		return nil
	}
	out := new(PaladinSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PluginConfig) DeepCopyInto(out *PluginConfig) {
	*out = *in
	if in.Class != nil {
		in, out := &in.Class, &out.Class
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PluginConfig.
func (in *PluginConfig) DeepCopy() *PluginConfig {
	if in == nil {
		return nil
	}
	out := new(PluginConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegistryReference) DeepCopyInto(out *RegistryReference) {
	*out = *in
	in.LabelReference.DeepCopyInto(&out.LabelReference)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegistryReference.
func (in *RegistryReference) DeepCopy() *RegistryReference {
	if in == nil {
		return nil
	}
	out := new(RegistryReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegistryTransportsConfig) DeepCopyInto(out *RegistryTransportsConfig) {
	*out = *in
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = new(bool)
		**out = **in
	}
	if in.TransportMap != nil {
		in, out := &in.TransportMap, &out.TransportMap
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegistryTransportsConfig.
func (in *RegistryTransportsConfig) DeepCopy() *RegistryTransportsConfig {
	if in == nil {
		return nil
	}
	out := new(RegistryTransportsConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretBackedSigner) DeepCopyInto(out *SecretBackedSigner) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretBackedSigner.
func (in *SecretBackedSigner) DeepCopy() *SecretBackedSigner {
	if in == nil {
		return nil
	}
	out := new(SecretBackedSigner)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SmartContractDeployment) DeepCopyInto(out *SmartContractDeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SmartContractDeployment.
func (in *SmartContractDeployment) DeepCopy() *SmartContractDeployment {
	if in == nil {
		return nil
	}
	out := new(SmartContractDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *SmartContractDeployment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SmartContractDeploymentList) DeepCopyInto(out *SmartContractDeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]SmartContractDeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SmartContractDeploymentList.
func (in *SmartContractDeploymentList) DeepCopy() *SmartContractDeploymentList {
	if in == nil {
		return nil
	}
	out := new(SmartContractDeploymentList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *SmartContractDeploymentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SmartContractDeploymentSpec) DeepCopyInto(out *SmartContractDeploymentSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SmartContractDeploymentSpec.
func (in *SmartContractDeploymentSpec) DeepCopy() *SmartContractDeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(SmartContractDeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SmartContractDeploymentStatus) DeepCopyInto(out *SmartContractDeploymentStatus) {
	*out = *in
	out.TransactionSubmission = in.TransactionSubmission
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SmartContractDeploymentStatus.
func (in *SmartContractDeploymentStatus) DeepCopy() *SmartContractDeploymentStatus {
	if in == nil {
		return nil
	}
	out := new(SmartContractDeploymentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Status) DeepCopyInto(out *Status) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Status.
func (in *Status) DeepCopy() *Status {
	if in == nil {
		return nil
	}
	out := new(Status)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSConfig) DeepCopyInto(out *TLSConfig) {
	*out = *in
	if in.AdditionalDNSNames != nil {
		in, out := &in.AdditionalDNSNames, &out.AdditionalDNSNames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSConfig.
func (in *TLSConfig) DeepCopy() *TLSConfig {
	if in == nil {
		return nil
	}
	out := new(TLSConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TransactionSubmission) DeepCopyInto(out *TransactionSubmission) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TransactionSubmission.
func (in *TransactionSubmission) DeepCopy() *TransactionSubmission {
	if in == nil {
		return nil
	}
	out := new(TransactionSubmission)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TransportConfig) DeepCopyInto(out *TransportConfig) {
	*out = *in
	in.Plugin.DeepCopyInto(&out.Plugin)
	in.TLS.DeepCopyInto(&out.TLS)
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]v1.ServicePort, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TransportConfig.
func (in *TransportConfig) DeepCopy() *TransportConfig {
	if in == nil {
		return nil
	}
	out := new(TransportConfig)
	in.DeepCopyInto(out)
	return out
}
