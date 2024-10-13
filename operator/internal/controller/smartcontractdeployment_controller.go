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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/google/uuid"
	"github.com/hyperledger/firefly-signer/pkg/abi"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/toolkit/pkg/ptxapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
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

//+kubebuilder:rbac:groups=core.paladin.io,resources=smartcontractdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.paladin.io,resources=smartcontractdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.paladin.io,resources=smartcontractdeployments/finalizers,verbs=update

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

	// If we don't have an idempotency key, then create one and re-reconcile
	if scd.Status.IdempotencyKey == "" {
		scd.Status.TransactionStatus = corev1alpha1.TransactionStatusSubmitting
		scd.Status.IdempotencyKey = fmt.Sprintf("k8s.%s.%d", scd.Name, scd.CreationTimestamp.UnixMilli())
		return r.updateStatusAndRequeue(ctx, &scd)
	}

	// Check availability of the Paladin node and deploy
	paladinRPC, err := r.getPaladinRPC(ctx, &scd)
	if err != nil {
		return ctrl.Result{}, err
	} else if paladinRPC == nil {
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// If we don't have a transactionID to track, then submit (moves us to Pending)
	if scd.Status.TransactionID == "" {
		return r.submitTransactionAndRequeue(ctx, &scd, paladinRPC)
	}

	// If we don't have a result
	if scd.Status.TransactionStatus == corev1alpha1.TransactionStatusPending {
		return r.trackTransactionAndRequeue(ctx, &scd, paladinRPC)
	}

	// Nothing left to do
	return ctrl.Result{}, nil
}

func (r *SmartContractDeploymentReconciler) updateStatusAndRequeue(ctx context.Context, scd *corev1alpha1.SmartContractDeployment) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, scd); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update smart contract deployment status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil // Run again immediately to submit
}

func (r *SmartContractDeploymentReconciler) submitTransactionAndRequeue(ctx context.Context, scd *corev1alpha1.SmartContractDeployment, paladinRPC rpcclient.Client) (ctrl.Result, error) {

	var data tktypes.RawJSON
	if scd.Spec.ParamsJSON == "" {
		data = tktypes.RawJSON(scd.Spec.ParamsJSON)
	}
	var a abi.ABI
	if err := json.Unmarshal([]byte(scd.Spec.ABI), &a); err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid ABI: %s", err)
	}
	bytecode, err := tktypes.ParseHexBytes(ctx, scd.Spec.Bytecode)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid bytecode: %s", err)
	}

	tx := &ptxapi.TransactionInput{
		Transaction: ptxapi.Transaction{
			IdempotencyKey: scd.Status.IdempotencyKey,
			Type:           tktypes.Enum[ptxapi.TransactionType](scd.Spec.TxType),
			Domain:         scd.Spec.Domain,
			From:           scd.Spec.DeployKey,
			Data:           data,
		},
		ABI:      a,
		Bytecode: bytecode,
	}

	var txID uuid.UUID
	err = paladinRPC.CallRPC(ctx, &txID, "ptx_sendTransaction", tx)
	if err != nil {
		if strings.Contains(err.Error(), "PD012220") {
			log.FromContext(ctx).Info(fmt.Sprintf("recovering TX by idempotencyKey: %s", err))
			return r.queryTxByIdempotencyKeyAndRequeue(ctx, scd, paladinRPC)
		}
		return ctrl.Result{}, err
	}
	scd.Status.TransactionID = txID.String()
	scd.Status.TransactionStatus = corev1alpha1.TransactionStatusPending
	return r.updateStatusAndRequeue(ctx, scd)

}

func (r *SmartContractDeploymentReconciler) queryTxByIdempotencyKeyAndRequeue(ctx context.Context, scd *corev1alpha1.SmartContractDeployment, paladinRPC rpcclient.Client) (ctrl.Result, error) {
	var txns []*ptxapi.Transaction
	err := paladinRPC.CallRPC(ctx, &txns, "ptx_queryTransactions",
		query.NewQueryBuilder().Equal("idempotencyKey", scd.Status.IdempotencyKey).Limit(1))
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(txns) == 0 {
		return ctrl.Result{}, fmt.Errorf("failed to query transaction with idempotencyKey '%s' after PD012220 error", scd.Status.IdempotencyKey)
	}
	scd.Status.TransactionID = txns[0].ID.String()
	scd.Status.TransactionStatus = corev1alpha1.TransactionStatusPending
	return r.updateStatusAndRequeue(ctx, scd)
}

func (r *SmartContractDeploymentReconciler) trackTransactionAndRequeue(ctx context.Context, scd *corev1alpha1.SmartContractDeployment, paladinRPC rpcclient.Client) (ctrl.Result, error) {
	var txReceipt *ptxapi.TransactionReceipt
	err := paladinRPC.CallRPC(ctx, &txReceipt, "ptx_getTransactionReceipt", scd.Status.TransactionID)
	if err != nil {
		return ctrl.Result{}, err
	}
	if txReceipt == nil {
		return ctrl.Result{
			RequeueAfter: 1 * time.Second,
		}, nil
	}
	if txReceipt.TransactionHash != nil {
		scd.Status.TransactionHash = txReceipt.TransactionHash.String()
	}
	if txReceipt.Success {
		if txReceipt.ContractAddress == nil {
			scd.Status.TransactionStatus = corev1alpha1.TransactionStatusFailed
			scd.Status.FailureMessage = "transaction did not result in contract deployment"
		} else {
			scd.Status.TransactionStatus = corev1alpha1.TransactionStatusSuccess
			scd.Status.ContractAddress = txReceipt.ContractAddress.String()
		}
	} else {
		scd.Status.TransactionStatus = corev1alpha1.TransactionStatusFailed
		scd.Status.FailureMessage = txReceipt.FailureMessage
	}
	return r.updateStatusAndRequeue(ctx, scd)
}

func (r *SmartContractDeploymentReconciler) getPaladinRPC(ctx context.Context, scd *corev1alpha1.SmartContractDeployment) (rpcclient.Client, error) {

	log := log.FromContext(ctx)
	var node corev1alpha1.Paladin
	if err := r.Get(ctx, types.NamespacedName{Name: scd.Spec.DeployNode, Namespace: scd.Namespace}, &node); err != nil {
		if errors.IsNotFound(err) {
			// short wait for node to become available
			log.Info(fmt.Sprintf("Waiting for paladin node '%s' to be created to deploy", scd.Spec.DeployNode))
			return nil, nil
		}
		// short wait for node to become available
		log.Info(fmt.Sprintf("Waiting for paladin node '%s' to become available to deploy", scd.Spec.DeployNode))
		return nil, nil
	}
	url, err := getPaladinURLEndpoint(ctx, r.Client, scd.Spec.DeployNode, scd.Namespace)
	if err != nil {
		return nil, err
	}
	return rpcclient.NewHTTPClient(ctx, &pldconf.HTTPClientConfig{URL: url})

}

// SetupWithManager sets up the controller with the Manager.
func (r *SmartContractDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SmartContractDeployment{}).
		Complete(r)
}
