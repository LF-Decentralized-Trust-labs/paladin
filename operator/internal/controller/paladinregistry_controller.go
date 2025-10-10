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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
)

// PaladinRegistryReconciler reconciles a PaladinRegistry object
type PaladinRegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var PaladinRegistryCRMap = CRMap[corev1alpha1.PaladinRegistry, *corev1alpha1.PaladinRegistry, *corev1alpha1.PaladinRegistryList]{
	NewList:  func() *corev1alpha1.PaladinRegistryList { return new(corev1alpha1.PaladinRegistryList) },
	ItemsFor: func(list *corev1alpha1.PaladinRegistryList) []corev1alpha1.PaladinRegistry { return list.Items },
	AsObject: func(item *corev1alpha1.PaladinRegistry) *corev1alpha1.PaladinRegistry { return item },
}

func (r *PaladinRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PaladinRegistry instance
	var reg corev1alpha1.PaladinRegistry
	if err := r.Get(ctx, req.NamespacedName, &reg); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PaladinRegistry registry")
		return ctrl.Result{}, err
	}

	if reg.Spec.Type == corev1alpha1.RegistryTypeEVM {
		if reg.Spec.EVM.ContractAddress != "" {
			// do we have a fixed contract address?
			if reg.Status.ContractAddress != reg.Spec.EVM.ContractAddress {
				reg.Status.ContractAddress = reg.Spec.EVM.ContractAddress
				reg.Status.Status = corev1alpha1.RegistryStatusAvailable
				return r.updateStatusAndRequeue(ctx, &reg)
			}
		} else if reg.Spec.EVM.SmartContractDeployment != "" {
			if reg.Status.Status == "" {
				reg.Status.Status = corev1alpha1.RegistryStatusPending
				return r.updateStatusAndRequeue(ctx, &reg)
			}
			if reg.Status.Status != corev1alpha1.RegistryStatusAvailable {
				return r.trackContractDeploymentAndRequeue(ctx, &reg)
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("missing contractAddress or smartContractDeployment")
		}
	}

	return ctrl.Result{}, nil
}

func (r *PaladinRegistryReconciler) reconcileSmartContractDeployment(ctx context.Context, obj client.Object) []ctrl.Request {
	scd, ok := obj.(*corev1alpha1.SmartContractDeployment)
	if !ok {
		log.FromContext(ctx).Error(fmt.Errorf("unexpected object type"), "expected SmartContractDeployment")
		return nil
	}

	if scd.Status.TransactionStatus != corev1alpha1.TransactionStatusSuccess {
		return nil
	}

	regs := &corev1alpha1.PaladinRegistryList{}
	if err := r.Client.List(ctx, regs, client.InNamespace(scd.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list Paladin registries")
		return nil
	}

	reqs := make([]ctrl.Request, 0, len(regs.Items))

	for _, reg := range regs.Items {
		if reg.Spec.EVM.ContractAddress != "" {
			if reg.Spec.EVM.ContractAddress != scd.Status.ContractAddress {
				continue
			}
		} else {
			if reg.Spec.EVM.SmartContractDeployment != scd.Name {
				continue
			}
		}
		reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&reg)})
	}
	return reqs
}

func (r *PaladinRegistryReconciler) updateStatusAndRequeue(ctx context.Context, reg *corev1alpha1.PaladinRegistry) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, reg); err != nil && !errors.IsConflict(err) {
		log.FromContext(ctx).Error(err, "Failed to update Paladin registry status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 50 * time.Millisecond}, nil // Run again immediately to submit
}

func (r *PaladinRegistryReconciler) trackContractDeploymentAndRequeue(ctx context.Context, reg *corev1alpha1.PaladinRegistry) (ctrl.Result, error) {

	var scd corev1alpha1.SmartContractDeployment
	err := r.Get(ctx, types.NamespacedName{Name: reg.Spec.EVM.SmartContractDeployment, Namespace: reg.Namespace}, &scd)
	if err != nil {
		if errors.IsNotFound(err) {
			log.FromContext(ctx).Info(fmt.Sprintf("Waiting for creation of smart contract deployment '%s'", scd.Name))
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	if scd.Status.ContractAddress == "" {
		log.FromContext(ctx).Info(fmt.Sprintf("Registry: '%s'. Waiting for successful deployment of smart contract deployment '%s'", reg.Name, scd.Name))
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	reg.Status.ContractAddress = scd.Status.ContractAddress
	reg.Status.Status = corev1alpha1.RegistryStatusAvailable
	return r.updateStatusAndRequeue(ctx, reg)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PaladinRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.PaladinRegistry{}).
		// Reconcile when related contract deployment changes status
		Watches(&corev1alpha1.SmartContractDeployment{}, handler.EnqueueRequestsFromMapFunc(r.reconcileSmartContractDeployment), reconcileEveryChange()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 2,
		}).
		Complete(r)
}
