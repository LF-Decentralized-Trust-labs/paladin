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
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hyperledger/firefly-signer/pkg/abi"
	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/tktypes"
)

// SmartContractDeploymentReconciler reconciles a SmartContractDeployment object
type SmartContractDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var SmartContractDeploymentCRMap = CRMap[corev1alpha1.SmartContractDeployment, *corev1alpha1.SmartContractDeployment, *corev1alpha1.SmartContractDeploymentList]{
	NewList: func() *corev1alpha1.SmartContractDeploymentList { return new(corev1alpha1.SmartContractDeploymentList) },
	ItemsFor: func(list *corev1alpha1.SmartContractDeploymentList) []corev1alpha1.SmartContractDeployment {
		return list.Items
	},
	AsObject: func(item *corev1alpha1.SmartContractDeployment) *corev1alpha1.SmartContractDeployment { return item },
}

func (r *SmartContractDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO: Add an admission webhook to make the bytecode and ABI immutable

	// Fetch the SmartContractDeployment instance
	var scd corev1alpha1.SmartContractDeployment
	if err := r.Get(ctx, req.NamespacedName, &scd); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SmartContractDeployment resource")
		return ctrl.Result{}, err
	}

	// Reconcile the deployment transaction
	txReconcile := newTransactionReconcile(r.Client,
		"scdeploy."+scd.Name,
		scd.Spec.DeployNode, scd.Namespace,
		&scd.Status.TransactionSubmission,
		func() (bool, *pldapi.TransactionInput, error) { return r.buildDeployTransaction(ctx, &scd) },
	)
	err := txReconcile.reconcile(ctx)
	if err != nil {
		// There's nothing to notify us when the world changes other than polling, so we keep re-trying
		return ctrl.Result{RequeueAfter: 1 * time.Second}, err
	} else if txReconcile.statusChanged {
		// Common TX reconciler does everything for us apart from grab the receipt
		if scd.Status.TransactionStatus == corev1alpha1.TransactionStatusSuccess && scd.Status.ContractAddress == "" {
			if txReconcile.receipt.ContractAddress == nil {
				scd.Status.TransactionStatus = corev1alpha1.TransactionStatusFailed
				scd.Status.FailureMessage = "transaction did not result in contract deployment"
			} else {
				scd.Status.ContractAddress = txReconcile.receipt.ContractAddress.String()
			}
		}
		return r.updateStatusAndRequeue(ctx, &scd)
	} else if !txReconcile.failed && !txReconcile.succeeded {
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}
	// Nothing left to do - we succeeded or failed
	return ctrl.Result{}, nil
}

func (r *SmartContractDeploymentReconciler) updateStatusAndRequeue(ctx context.Context, scd *corev1alpha1.SmartContractDeployment) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, scd); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update smart contract deployment status")
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, err
	}
	return ctrl.Result{Requeue: true}, nil // Run again immediately to submit
}

func (r *SmartContractDeploymentReconciler) buildDeployTransaction(ctx context.Context, scd *corev1alpha1.SmartContractDeployment) (bool, *pldapi.TransactionInput, error) {
	var data tktypes.RawJSON
	if scd.Spec.ParamsJSON == "" {
		data = tktypes.RawJSON(scd.Spec.ParamsJSON)
	}
	var a abi.ABI
	if err := json.Unmarshal([]byte(scd.Spec.ABI), &a); err != nil {
		return false, nil, fmt.Errorf("invalid ABI: %s", err)
	}
	bytecode, err := tktypes.ParseHexBytes(ctx, scd.Spec.Bytecode)
	if err != nil {
		return false, nil, fmt.Errorf("invalid bytecode: %s", err)
	}

	return true, &pldapi.TransactionInput{
		Transaction: pldapi.Transaction{
			Type:   tktypes.Enum[pldapi.TransactionType](scd.Spec.TxType),
			Domain: scd.Spec.Domain,
			From:   scd.Spec.DeployKey,
			Data:   data,
		},
		ABI:      a,
		Bytecode: bytecode,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SmartContractDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SmartContractDeployment{}).
		// Reconcile when any node status changes
		Watches(&corev1alpha1.Paladin{}, reconcileAll(SmartContractDeploymentCRMap, r.Client), reconcileEveryChange()).
		Complete(r)
}
