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

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
)

// PaladinDomainReconciler reconciles a PaladinDomain object
type PaladinDomainReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var PaladinDomainCRMap = CRMap[corev1alpha1.PaladinDomain, *corev1alpha1.PaladinDomain, *corev1alpha1.PaladinDomainList]{
	NewList:  func() *corev1alpha1.PaladinDomainList { return new(corev1alpha1.PaladinDomainList) },
	ItemsFor: func(list *corev1alpha1.PaladinDomainList) []corev1alpha1.PaladinDomain { return list.Items },
	AsObject: func(item *corev1alpha1.PaladinDomain) *corev1alpha1.PaladinDomain { return item },
}

func (r *PaladinDomainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PaladinDomain instance
	var domain corev1alpha1.PaladinDomain
	if err := r.Get(ctx, req.NamespacedName, &domain); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PaladinDomain registry")
		return ctrl.Result{}, err
	}

	if domain.Status.Status == "" {
		domain.Status.Status = corev1alpha1.DomainStatusPending
		return r.updateStatusAndRequeue(ctx, &domain)
	} else if domain.Status.Status == corev1alpha1.DomainStatusPending {
		if domain.Spec.RegistryAddress != "" {
			domain.Status.RegistryAddress = domain.Spec.RegistryAddress
			domain.Status.Status = corev1alpha1.DomainStatusAvailable
			return r.updateStatusAndRequeue(ctx, &domain)
		} else if domain.Spec.SmartContractDeployment != "" {
			return r.trackContractDeploymentAndRequeue(ctx, &domain)
		} else {
			return ctrl.Result{}, fmt.Errorf("missing registryAddress or smartContractDeployment")
		}

	}

	return ctrl.Result{}, nil
}

func (r *PaladinDomainReconciler) updateStatusAndRequeue(ctx context.Context, domain *corev1alpha1.PaladinDomain) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, domain); err != nil && !errors.IsConflict(err) {
		log.FromContext(ctx).Error(err, "Failed to update Paladin domain status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 50 * time.Millisecond}, nil // Run again immediately to submit
}

func (r *PaladinDomainReconciler) trackContractDeploymentAndRequeue(ctx context.Context, domain *corev1alpha1.PaladinDomain) (ctrl.Result, error) {

	var scd corev1alpha1.SmartContractDeployment
	err := r.Get(ctx, types.NamespacedName{Name: domain.Spec.SmartContractDeployment, Namespace: domain.Namespace}, &scd)
	if err != nil {
		if errors.IsNotFound(err) {
			log.FromContext(ctx).Info(fmt.Sprintf("Waiting for creation of smart contract deployment '%s'", scd.Name))
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	if scd.Status.ContractAddress == "" {
		log.FromContext(ctx).Info(fmt.Sprintf("Domain: '%s'. Waiting for successful deployment of smart contract deployment '%s'", domain.Name, scd.Name))
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	domain.Status.RegistryAddress = scd.Status.ContractAddress
	domain.Status.Status = corev1alpha1.DomainStatusAvailable
	return r.updateStatusAndRequeue(ctx, domain)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PaladinDomainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.PaladinDomain{}).
		// Reconcile when any contract deployment changes status
		Watches(&corev1alpha1.SmartContractDeployment{}, reconcileAll(PaladinDomainCRMap, r.Client), reconcileEveryChange()).
		Complete(r)
}
