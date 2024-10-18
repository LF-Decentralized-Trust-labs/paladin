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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/google/uuid"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldapi"
	"github.com/kaleido-io/paladin/toolkit/pkg/pldclient"
	"github.com/kaleido-io/paladin/toolkit/pkg/query"
	"github.com/kaleido-io/paladin/toolkit/pkg/rpcclient"
)

type transactionReconcile struct {
	client.Client
	idempotencyKeyPrefix string
	nodeName             string
	namespace            string
	pStatus              *corev1alpha1.TransactionSubmission
	txFactory            func() (bool, *pldapi.TransactionInput, error)
	receipt              *pldapi.TransactionReceipt
	statusChanged        bool
	succeeded            bool
	failed               bool
}

func newTransactionReconcile(c client.Client,
	idempotencyKeyPrefix,
	nodeName, namespace string,
	pStatus *corev1alpha1.TransactionSubmission,
	txFactory func() (bool, *pldapi.TransactionInput, error),
) *transactionReconcile {
	return &transactionReconcile{
		Client:               c,
		idempotencyKeyPrefix: idempotencyKeyPrefix,
		nodeName:             nodeName,
		namespace:            namespace,
		txFactory:            txFactory,
		pStatus:              pStatus,
	}
}

func (r *transactionReconcile) reconcile(ctx context.Context) error {

	// If we're already done, just return as such
	if r.pStatus.TransactionStatus == corev1alpha1.TransactionStatusFailed {
		r.failed = true
		return nil
	}
	if r.pStatus.TransactionStatus == corev1alpha1.TransactionStatusSuccess {
		r.succeeded = true
		return nil
	}

	// If we don't have an idempotency key, then create one and re-reconcile
	if r.pStatus.IdempotencyKey == "" {
		r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusSubmitting
		r.pStatus.IdempotencyKey = fmt.Sprintf("k8s.%s.%d", r.idempotencyKeyPrefix, time.Now().UnixMicro())
		r.statusChanged = true
		return nil
	}

	// Check availability of the Paladin node and deploy
	paladinRPC, err := getPaladinRPC(ctx, r.Client, r.nodeName, r.namespace)
	if err != nil || paladinRPC == nil {
		return err
	}

	// If we don't have a transactionID to track, then submit (moves us to Pending)
	if r.pStatus.TransactionID == "" {
		return r.submitTransactionAndRequeue(ctx, paladinRPC)
	}

	// We're tracking for completion
	return r.trackTransactionAndRequeue(ctx, paladinRPC)
}

func (r *transactionReconcile) submitTransactionAndRequeue(ctx context.Context, paladinRPC rpcclient.Client) error {

	ready, tx, err := r.txFactory()
	if err != nil {
		return err
	} else if !ready {
		log.FromContext(ctx).Info(fmt.Sprintf("waiting for pre-reqs before submitting TX %s", r.pStatus.IdempotencyKey))
		return nil
	}
	tx.IdempotencyKey = r.pStatus.IdempotencyKey

	var txID uuid.UUID
	err = paladinRPC.CallRPC(ctx, &txID, "ptx_sendTransaction", tx)
	if err != nil {
		if strings.Contains(err.Error(), "PD012220") {
			log.FromContext(ctx).Info(fmt.Sprintf("recovering TX by idempotencyKey: %s", err))
			return r.queryTxByIdempotencyKeyAndRequeue(ctx, paladinRPC)
		}
		return err
	}
	r.pStatus.TransactionID = txID.String()
	r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusPending
	r.statusChanged = true
	return nil

}

func (r *transactionReconcile) queryTxByIdempotencyKeyAndRequeue(ctx context.Context, paladinRPC rpcclient.Client) error {
	var txns []*pldapi.Transaction
	err := paladinRPC.CallRPC(ctx, &txns, "ptx_queryTransactions",
		query.NewQueryBuilder().Equal("idempotencyKey", r.pStatus.IdempotencyKey).Limit(1))
	if err != nil {
		return err
	}
	if len(txns) == 0 {
		return fmt.Errorf("failed to query transaction with idempotencyKey '%s' after PD012220 error", r.pStatus.IdempotencyKey)
	}
	r.pStatus.TransactionID = txns[0].ID.String()
	r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusPending
	r.statusChanged = true
	return nil
}

func (r *transactionReconcile) trackTransactionAndRequeue(ctx context.Context, paladinRPC rpcclient.Client) error {
	err := paladinRPC.CallRPC(ctx, &r.receipt, "ptx_getTransactionReceipt", r.pStatus.TransactionID)
	if err != nil {
		return err
	}
	if r.receipt == nil {
		// waiting for the receipt
		return nil
	}
	if r.receipt.TransactionHash != nil {
		r.pStatus.TransactionHash = r.receipt.TransactionHash.String()
	}
	if r.receipt.Success {
		r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusSuccess
	} else {
		r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusFailed
		r.pStatus.FailureMessage = r.receipt.FailureMessage
	}
	r.statusChanged = true
	return nil
}

func getPaladinRPC(ctx context.Context, c client.Client, nodeName, namespace string) (pldclient.PaladinClient, error) {

	log := log.FromContext(ctx)
	var node corev1alpha1.Paladin
	if err := c.Get(ctx, types.NamespacedName{Name: nodeName, Namespace: namespace}, &node); err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Waiting for paladin node '%s' to be created to deploy", nodeName))
			return nil, nil
		}
		log.Info(fmt.Sprintf("Waiting for paladin node '%s' to become available to deploy", nodeName))
		return nil, nil
	}
	ready := node.Status.Phase == corev1alpha1.StatusPhaseCompleted
	if !ready {
		log.Info(fmt.Sprintf("Waiting for paladin node '%s' to reach completed phase (%s)", nodeName, node.Status.Phase))
		return nil, nil
	}

	url, err := getPaladinURLEndpoint(ctx, c, nodeName, namespace)
	if err != nil {
		return nil, err
	}
	return pldclient.New().HTTP(ctx, &pldconf.HTTPClientConfig{URL: url})

}
