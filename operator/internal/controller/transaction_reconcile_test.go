package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldclient"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/rpcclient"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ transactionReconcileInterface = &mockTransactionReconcile{}

type mockTransactionReconcile struct {
	pStatus           *corev1alpha1.TransactionSubmission
	reconcileFunc     func(ctx context.Context) error
	statusChangedFlag bool
	succeededFlag     bool
	failedFlag        bool
	receiptValue      *pldapi.TransactionReceipt
}

func (m *mockTransactionReconcile) reconcile(ctx context.Context) error {
	if m.pStatus != nil {
		if m.succeededFlag {
			m.pStatus.TransactionStatus = corev1alpha1.TransactionStatusSuccess
		} else if m.failedFlag {
			m.pStatus.TransactionStatus = corev1alpha1.TransactionStatusFailed
			m.pStatus.FailureMessage = "Mock failure"
		} else {
			m.pStatus.TransactionStatus = corev1alpha1.TransactionStatusPending
		}
		m.pStatus.TransactionHash = "0xhash"
	}
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx)
	}
	return nil
}

func (m *mockTransactionReconcile) isStatusChanged() bool {
	return m.statusChangedFlag
}

func (m *mockTransactionReconcile) isSucceeded() bool {
	return m.succeededFlag
}

func (m *mockTransactionReconcile) isFailed() bool {
	return m.failedFlag
}

func (m *mockTransactionReconcile) getReceipt() *pldapi.TransactionReceipt {
	return m.receiptValue
}
func setupTestTransactionReconcile(objs ...runtime.Object) (*transactionReconcile, client.Client, error) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}

	err = appsv1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}
	err = corev1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&corev1alpha1.Paladin{}).
		Build()

	tr := &transactionReconcile{
		Client:  fakeClient,
		pStatus: &corev1alpha1.TransactionSubmission{},
	}

	return tr, fakeClient, nil
}
func TestTransactionReconcile_AlreadySucceeded(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.TransactionStatus = corev1alpha1.TransactionStatusSuccess

	err = tr.reconcile(context.Background())
	require.NoError(t, err)
	assert.True(t, tr.succeeded)
	assert.False(t, tr.failed)
}
func TestTransactionReconcile_AlreadyFailed(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.TransactionStatus = corev1alpha1.TransactionStatusFailed

	err = tr.reconcile(context.Background())
	require.NoError(t, err)
	assert.True(t, tr.failed)
	assert.False(t, tr.succeeded)
}
func TestTransactionReconcile_NoIdempotencyKey(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.idempotencyKeyPrefix = "test"
	tr.pStatus.IdempotencyKey = ""

	err = tr.reconcile(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, tr.pStatus.IdempotencyKey)
	assert.Equal(t, corev1alpha1.TransactionStatusSubmitting, tr.pStatus.TransactionStatus)
	assert.True(t, tr.statusChanged)
}
func TestTransactionReconcile_PaladinNodeNotReady(t *testing.T) {
	tr, fakeClient, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.idempotencyKeyPrefix = "test"
	tr.pStatus.IdempotencyKey = "test-key"
	tr.nodeName = "test-node"
	tr.namespace = "default"

	// Mock Paladin node that is not ready
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhasePending,
		},
	}
	err = fakeClient.Create(context.Background(), paladin)
	require.NoError(t, err)

	tr.getPaladinRPCFunc = func(ctx context.Context, c client.Client, r *rpcClientManager, nodeName string, namespace string, timeout string) (pldclient.PaladinClient, error) {
		return nil, nil // Node not ready
	}

	err = tr.reconcile(context.Background())
	require.NoError(t, err)
	assert.False(t, tr.succeeded)
	assert.False(t, tr.failed)
	// Should return nil without changing status
	assert.False(t, tr.statusChanged)
}
func TestSubmitTransactionAndRequeue_Success(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.IdempotencyKey = "test-key"

	// Mock txFactory
	tr.txFactory = func() (bool, *pldapi.TransactionInput, error) {
		txInput := &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From: "0x123",
			},
		}
		return true, txInput, nil
	}

	// Mock paladinRPC
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			if method == "ptx_sendTransaction" {
				txID := uuid.New()
				*(result.(*uuid.UUID)) = txID
			}
			return nil
		},
	}

	err = tr.submitTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	assert.NotEmpty(t, tr.pStatus.TransactionID)
	assert.Equal(t, corev1alpha1.TransactionStatusPending, tr.pStatus.TransactionStatus)
	assert.True(t, tr.statusChanged)
}
func TestSubmitTransactionAndRequeue_Success_________2(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.IdempotencyKey = "test-key"

	// Mock txFactory
	tr.txFactory = func() (bool, *pldapi.TransactionInput, error) {
		txInput := &pldapi.TransactionInput{
			TransactionBase: pldapi.TransactionBase{
				From: "0x123",
			},
		}
		return true, txInput, nil
	}

	// Mock paladinRPC
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			if method == "ptx_sendTransaction" {
				txID := uuid.New()
				*(result.(*uuid.UUID)) = txID
			}
			return nil
		},
	}

	err = tr.submitTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	assert.NotEmpty(t, tr.pStatus.TransactionID)
	assert.Equal(t, corev1alpha1.TransactionStatusPending, tr.pStatus.TransactionStatus)
	assert.True(t, tr.statusChanged)
}

func TestSubmitTransactionAndRequeue_NotReady(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.IdempotencyKey = "test-key"

	// Mock txFactory
	tr.txFactory = func() (bool, *pldapi.TransactionInput, error) {
		return false, nil, nil // Not ready
	}

	paladinRPC := &mockRPCClient{}

	err = tr.submitTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	// Should not change status
	assert.False(t, tr.statusChanged)
}

func TestSubmitTransactionAndRequeue_Error(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.IdempotencyKey = "test-key"

	// Mock txFactory
	tr.txFactory = func() (bool, *pldapi.TransactionInput, error) {
		return true, nil, errors.New("txFactory error")
	}

	paladinRPC := &mockRPCClient{}

	err = tr.submitTransactionAndRequeue(context.Background(), paladinRPC)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "txFactory error")
}
func TestQueryTxByIdempotencyKeyAndRequeue_Success(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.IdempotencyKey = "test-key"

	// Mock paladinRPC
	u := uuid.New()
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			if method == "ptx_queryTransactions" {
				txns := []*pldapi.Transaction{
					{
						ID: &u,
					},
				}
				*(result.(*[]*pldapi.Transaction)) = txns
			}
			return nil
		},
	}

	err = tr.queryTxByIdempotencyKeyAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	assert.NotEmpty(t, tr.pStatus.TransactionID)
	assert.Equal(t, corev1alpha1.TransactionStatusPending, tr.pStatus.TransactionStatus)
	assert.True(t, tr.statusChanged)
}
func TestTrackTransactionAndRequeue_ReceiptNotAvailable(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.TransactionID = uuid.New().String()

	// Mock paladinRPC
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			if method == "ptx_getTransactionReceipt" {
				result = nil // Receipt not available
			}
			return nil
		},
	}

	err = tr.trackTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	// Should not change status
	assert.False(t, tr.statusChanged)
}
func TestTrackTransactionAndRequeue_Success(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.TransactionID = uuid.New().String()

	// Mock paladinRPC
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			tx := pldtypes.NewBytes32FromSlice([]byte("0xabc123"))
			if method == "ptx_getTransactionReceipt" {
				receipt := &pldapi.TransactionReceipt{
					TransactionReceiptData: pldapi.TransactionReceiptData{
						Success: true,
						TransactionReceiptDataOnchain: &pldapi.TransactionReceiptDataOnchain{
							TransactionHash: &tx,
						},
						ContractAddress: pldtypes.MustEthAddress("0x3078616263646566313233343536373839300000"),
						FailureMessage:  "",
					},
				}
				*(result.(**pldapi.TransactionReceipt)) = receipt
			}
			return nil
		},
	}

	err = tr.trackTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)

	// Call reconcile to update tr.succeeded
	err = tr.reconcile(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "0x3078616263313233000000000000000000000000000000000000000000000000", tr.pStatus.TransactionHash)
	assert.Equal(t, corev1alpha1.TransactionStatusSuccess, tr.pStatus.TransactionStatus)
	assert.True(t, tr.statusChanged)
	assert.True(t, tr.succeeded)
}

func TestTrackTransactionAndRequeue_Failure(t *testing.T) {
	tr, _, err := setupTestTransactionReconcile()
	require.NoError(t, err)

	tr.pStatus.TransactionID = uuid.New().String()

	// Mock paladinRPC
	paladinRPC := &mockRPCClient{
		callRPCFunc: func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
			if method == "ptx_getTransactionReceipt" {
				tx := pldtypes.NewBytes32FromSlice([]byte("0xabc123"))
				receipt := &pldapi.TransactionReceipt{
					TransactionReceiptData: pldapi.TransactionReceiptData{
						Success: false,
						TransactionReceiptDataOnchain: &pldapi.TransactionReceiptDataOnchain{
							TransactionHash: &tx,
						},
						FailureMessage: "Transaction failed",
					},
				}
				*(result.(**pldapi.TransactionReceipt)) = receipt
			}
			return nil
		},
	}

	err = tr.trackTransactionAndRequeue(context.Background(), paladinRPC)
	require.NoError(t, err)
	assert.Equal(t, "0x3078616263313233000000000000000000000000000000000000000000000000", tr.pStatus.TransactionHash)
	assert.Equal(t, corev1alpha1.TransactionStatusFailed, tr.pStatus.TransactionStatus)
	assert.Equal(t, "Transaction failed", tr.pStatus.FailureMessage)
	assert.True(t, tr.statusChanged)
	assert.True(t, tr.failed)
}
func TestGetPaladinRPC_NodeNotFound(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ctx := context.Background()

	paladinRPC, err := getPaladinRPC(ctx, fakeClient, nil, "non-existent-node", "default", "1s")
	require.NoError(t, err)
	assert.Nil(t, paladinRPC)
}
func TestGetPaladinRPC_NodeNotReady(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhasePending,
		},
	}
	_, fakeClient, _ := setupTestTransactionReconcile(paladin)

	ctx := context.Background()

	paladinRPC, err := getPaladinRPC(ctx, fakeClient, nil, "test-node", "default", "1s")
	require.NoError(t, err)
	assert.Nil(t, paladinRPC)
}

func TestGetPaladinRPC_Success(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhaseReady,
		},
	}
	sfs := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "paladin-test-node",
			Namespace: "default",
		},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      1,
		},
	}

	_, fakeClient, _ := setupTestTransactionReconcile(paladin, sfs)
	ctx := context.Background()

	// Mock getPaladinURLEndpointFunc
	getPaladinURLEndpointFunc = func(ctx context.Context, c client.Client, nodeName, namespace string) (string, error) {
		return "http://paladin-url", nil
	}
	defer func() {
		getPaladinURLEndpointFunc = getPaladinURLEndpoint // Reset after test
	}()

	r := NewRPCCache()
	paladinClient, err := getPaladinRPC(ctx, fakeClient, r, "test-node", "default", "1s")
	require.NoError(t, err)
	assert.NotNil(t, paladinClient)
}

type mockRPCClient struct {
	callRPCFunc func(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC
}

func (m *mockRPCClient) CallRPC(ctx context.Context, result interface{}, method string, args ...interface{}) rpcclient.ErrorRPC {
	if m.callRPCFunc != nil {
		return m.callRPCFunc(ctx, result, method, args...)
	}
	return nil
}
