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
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/confutil"
	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/query"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"
	"github.com/google/uuid"
)

var _ transactionReconcileInterface = &transactionReconcile{}

type transactionReconcileInterface interface {
	reconcile(ctx context.Context) error
	isStatusChanged() bool
	isSucceeded() bool
	isFailed() bool
	getReceipt() *pldapi.TransactionReceipt
}

type rpcClientManager struct {
	clients map[string]pldclient.PaladinClient
	mux     sync.RWMutex
}

func NewRPCCache() *rpcClientManager {
	return &rpcClientManager{
		clients: make(map[string]pldclient.PaladinClient),
	}
}

type transactionReconcile struct {
	client.Client
	rpcClientManager     *rpcClientManager
	idempotencyKeyPrefix string
	nodeName             string
	namespace            string
	pStatus              *corev1alpha1.TransactionSubmission
	txFactory            func() (bool, *pldapi.TransactionInput, error)
	receipt              *pldapi.TransactionReceipt
	statusChanged        bool
	succeeded            bool
	failed               bool
	getPaladinRPCFunc    func(context.Context, client.Client, *rpcClientManager, string, string, string) (pldclient.PaladinClient, error)
	timeout              string
}

func newTransactionReconcile(c client.Client,
	rpcClientManager *rpcClientManager,
	idempotencyKeyPrefix,
	nodeName, namespace string,
	pStatus *corev1alpha1.TransactionSubmission,
	timeout string,
	txFactory func() (bool, *pldapi.TransactionInput, error),
) transactionReconcileInterface {
	return &transactionReconcile{
		Client:               c,
		idempotencyKeyPrefix: idempotencyKeyPrefix,
		nodeName:             nodeName,
		namespace:            namespace,
		txFactory:            txFactory,
		pStatus:              pStatus,
		timeout:              timeout,
		rpcClientManager:     rpcClientManager,
	}
}
func (r *transactionReconcile) isStatusChanged() bool                  { return r.statusChanged }
func (r *transactionReconcile) isSucceeded() bool                      { return r.succeeded }
func (r *transactionReconcile) isFailed() bool                         { return r.failed }
func (r *transactionReconcile) getReceipt() *pldapi.TransactionReceipt { return r.receipt }

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

	if r.getPaladinRPCFunc == nil {
		r.getPaladinRPCFunc = getPaladinRPC
	}

	paladinRPC, err := r.getPaladinRPCFunc(ctx, r.Client, r.rpcClientManager, r.nodeName, r.namespace, r.timeout)
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
		query.NewQueryBuilder().Equal("idempotencyKey", r.pStatus.IdempotencyKey).Limit(1).Query())
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
		r.succeeded = true
	} else {
		r.pStatus.TransactionStatus = corev1alpha1.TransactionStatusFailed
		r.pStatus.FailureMessage = r.receipt.FailureMessage
		r.failed = true
	}
	r.statusChanged = true
	return nil
}

var getPaladinURLEndpointFunc = getPaladinURLEndpoint

func getPaladinRPC(ctx context.Context, c client.Client, rpcM *rpcClientManager, nodeName, namespace string, timeout string) (pldclient.PaladinClient, error) {

	log := log.FromContext(ctx)
	pName := generatePaladinName(nodeName)
	var pNode appsv1.StatefulSet
	if err := c.Get(ctx, types.NamespacedName{Name: pName, Namespace: namespace}, &pNode); err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Waiting for paladin node '%s' to be created to deploy", pName))
			return nil, nil
		}
		log.Info(fmt.Sprintf("Waiting for paladin node '%s' to become available to deploy", pName))
		return nil, nil
	}
	ready := pNode.Status.ReadyReplicas == pNode.Status.Replicas
	if !ready {
		log.Info(fmt.Sprintf("Waiting for paladin node '%s' to reach ready state (%d)", pName, pNode.Status.ReadyReplicas))
		return nil, nil
	}

	url, err := getPaladinURLEndpointFunc(ctx, c, nodeName, namespace)
	if err != nil {
		return nil, err
	}

	// Adding the timeout to the cache key to ensure that different timeouts are cached separately
	// This is important because the timeout is used in the HTTP client config
	// and different timeouts may require different configurations.
	key := fmt.Sprintf("%s/%s", nodeName, timeout)

	// Check if the client is already in the cache
	// Use a read lock to avoid blocking other goroutines
	rpcM.mux.RLock()
	if client, ok := rpcM.clients[key]; ok && client != nil {
		rpcM.mux.RUnlock()
		return client, nil
	}
	rpcM.mux.RUnlock()

	// If not, create a new client and store it in the cache
	rpcM.mux.Lock()
	defer rpcM.mux.Unlock()

	// Check again in the cache after acquiring the lock
	// This is to ensure that another goroutine didn't create the client while we were waiting for the lock
	if client, ok := rpcM.clients[key]; ok && client != nil {
		return client, nil
	}

	client, err := pldclient.New().HTTP(ctx, &pldconf.HTTPClientConfig{
		URL:               url,
		ConnectionTimeout: confutil.P(timeout),
		RequestTimeout:    confutil.P(timeout),
	})
	if err != nil {
		return nil, err
	}

	rpcM.clients[key] = client
	return client, nil
}

func (r *rpcClientManager) removeNode(nodeName string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	var toRemove []string
	for i := range r.clients {
		if strings.HasPrefix(i, nodeName+"/") {
			toRemove = append(toRemove, i)
		}
	}
	for _, i := range toRemove {
		delete(r.clients, i) // Remove the client from the cache
	}
}
