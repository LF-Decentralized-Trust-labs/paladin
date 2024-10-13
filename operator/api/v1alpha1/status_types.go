package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatusPhase string

const (
	StatusPhasePending   StatusPhase = "Pending"
	StatusPhaseCompleted StatusPhase = "Completed"
	StatusPhaseFailed    StatusPhase = "Failed"
	StatusPhaseUnknown   StatusPhase = "Unknown"
)

type ConditionType string

const (

	// generic conditions
	ConditionCM     ConditionType = "ConfigMap"
	ConditionSS     ConditionType = "StatefulSet"
	ConditionSecret ConditionType = "Secret"
	ConditionSVC    ConditionType = "Service"
	ConditionPDB    ConditionType = "PodDisruptionBudget"
	ConditionPVC    ConditionType = "PersistentVolumeClaim"

	ConditionGenesisAvailable ConditionType = "GenesisAvailable"
)

// const (
// 	ConditionDependentResources ConditionType = "DependentResourcesReady"
// )

type ConditionReason string

const (
	// failed
	ReasonCMCreationFailed     ConditionReason = "ConfigMapCreationFailed"
	ReasonSSCreationFailed     ConditionReason = "StatefulSetCreationFailed"
	ReasonSecretCreationFailed ConditionReason = "SecretCreationFailed"
	ReasonSVCCreationFailed    ConditionReason = "ServiceCreationFailed"
	ReasonPDBCreationFailed    ConditionReason = "PodDisruptionBudgetCreationFailed"
	ReasonPVCCreationFailed    ConditionReason = "PersistentVolumeClaimCreationFailed"

	// created
	ReasonCMCreated     ConditionReason = "ConfigMapCreated"
	ReasonSSCreated     ConditionReason = "StatefulSetCreated"
	ReasonSecretCreated ConditionReason = "SecretCreated"
	ReasonSVCCreated    ConditionReason = "ServiceCreated"
	ReasonPDBCreated    ConditionReason = "PodDisruptionBudgetCreated"
	ReasonPVCCreated    ConditionReason = "PersistentVolumeClaimCreated"

	// updated
	ReasonCMUpdated     ConditionReason = "ConfigMapUpdated"
	ReasonSSUpdated     ConditionReason = "StatefulSetUpdated"
	ReasonSecretUpdated ConditionReason = "SecretUpdated"
	ReasonSVCUpdated    ConditionReason = "ServiceUpdated"
	ReasonPDBUpdated    ConditionReason = "PodDisruptionBudgetUpdated"

	ReasonSuccess         ConditionReason = "Success"
	ReasonGenesisNotFound ConditionReason = "GenesisNotFound"
)

// Status defines the observed state of a given object
type Status struct {
	// Phase represents the current phase of the Besu resource
	Phase StatusPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the Besu's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
