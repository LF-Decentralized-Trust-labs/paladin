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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *PaladinRegistry) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-core-paladin-io-v1alpha1-paladinregistry,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.paladin.io,resources=paladinregistries,verbs=create;update;delete,versions=v1alpha1,name=vpaladinregistry.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &PaladinRegistry{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *PaladinRegistry) ValidateCreate() (admission.Warnings, error) {
	// allow creation
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *PaladinRegistry) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	// Deny any update
	return nil, fmt.Errorf("updates to PaladinRegistry resources are not allowed as they have already been submitted to the blockchain and cannot be modified")
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *PaladinRegistry) ValidateDelete() (admission.Warnings, error) {
	// Deny any delete
	return nil, fmt.Errorf("deletions of PaladinRegistry resources are not allowed as they have already been submitted to the blockchain and cannot be deleted")
}
