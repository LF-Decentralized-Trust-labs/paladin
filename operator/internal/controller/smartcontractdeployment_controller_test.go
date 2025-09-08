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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldapi"
	"github.com/LF-Decentralized-Trust-labs/paladin/sdk/go/pkg/pldtypes"
)

var _ = Describe("SmartContractDeployment Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		smartcontractdeployment := &corev1alpha1.SmartContractDeployment{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SmartContractDeployment")
			err := k8sClient.Get(ctx, typeNamespacedName, smartcontractdeployment)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.SmartContractDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.SmartContractDeploymentSpec{
						TxType: "public",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.SmartContractDeployment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SmartContractDeployment")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SmartContractDeploymentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

func setupSmartContractDeploymentTestReconciler(objs ...runtime.Object) (*SmartContractDeploymentReconciler, error) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = corev1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&corev1alpha1.SmartContractDeployment{}). // Handle status updates
		Build()

	reconciler := &SmartContractDeploymentReconciler{
		Client: client,
		Scheme: scheme,
	}

	return reconciler, nil
}

func TestSmartContractDeploymentReconcile_Success(t *testing.T) {
	// Set up logger for testing
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a SmartContractDeployment resource
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			Node:     "test-node",
			TxType:   "public",
			From:     "0x123",
			ABIJSON:  `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"}]`,
			Bytecode: "0x6003600501",
		},
	}

	reconciler, err := setupSmartContractDeploymentTestReconciler(scd)
	require.NoError(t, err)

	// Inject mocked dependencies
	reconciler.checkDepsFunc = func(_ context.Context, _ client.Client, _ string, _ []string, _ *corev1alpha1.ContactDependenciesStatus) (bool, bool, error) {
		return false, true, nil
	}

	// Mock transaction reconcile
	reconciler.newTransactionReconcileFunc = func(c client.Client, r *rpcClientManager, idempotencyKeyPrefix string, nodeName string, namespace string, pStatus *corev1alpha1.TransactionSubmission, timeout string, txFactory func() (bool, *pldapi.TransactionInput, error)) transactionReconcileInterface {
		return &mockTransactionReconcile{
			pStatus:           pStatus,
			statusChangedFlag: true,
			succeededFlag:     true,
			receiptValue: &pldapi.TransactionReceipt{
				TransactionReceiptData: pldapi.TransactionReceiptData{
					ContractAddress: pldtypes.MustEthAddress("0x3078616263646566313233343536373839300000"),
				},
			},
		}
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-scd",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated SmartContractDeployment
	updatedScd := &corev1alpha1.SmartContractDeployment{}
	err = reconciler.Get(ctx, req.NamespacedName, updatedScd)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.TransactionStatusSuccess, updatedScd.Status.TransactionStatus)
	assert.Equal(t, "0x3078616263646566313233343536373839300000", updatedScd.Status.ContractAddress)
}

func TestSmartcontractDeploymentUpdateStatusAndRequeue(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			TransactionSubmission: corev1alpha1.TransactionSubmission{
				TransactionStatus: corev1alpha1.TransactionStatusPending,
			},
		},
	}

	reconciler, err := setupSmartContractDeploymentTestReconciler(scd)
	require.NoError(t, err)

	ctx := context.Background()

	scd.Status.TransactionStatus = corev1alpha1.TransactionStatusSuccess
	scd.Status.ContractAddress = "0xabcdef1234567890"
	result, err := reconciler.updateStatusAndRequeue(ctx, scd)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated SmartContractDeployment
	updatedScd := &corev1alpha1.SmartContractDeployment{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      "test-scd",
		Namespace: "default",
	}, updatedScd)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.TransactionStatusSuccess, updatedScd.Status.TransactionStatus)
	assert.Equal(t, "0xabcdef1234567890", updatedScd.Status.ContractAddress)
}
func TestBuildDeployTransaction_InvalidABI(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			ABIJSON:  "invalid-abi",
			Bytecode: "0x6003600501",
		},
	}

	reconciler := &SmartContractDeploymentReconciler{}

	_, _, err := reconciler.buildDeployTransaction(context.Background(), scd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ABI")
}

func TestBuildDeployTransaction_Success(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			ABIJSON:  `[{"inputs":[],"stateMutability":"nonpayable","type":"constructor"}]`,
			Bytecode: "0x6003600501",
			TxType:   "public",
			From:     "0x123",
		},
	}

	reconciler := &SmartContractDeploymentReconciler{}

	ready, tx, err := reconciler.buildDeployTransaction(context.Background(), scd)
	require.NoError(t, err)
	assert.True(t, ready)
	assert.NotNil(t, tx)
	assert.Equal(t, scd.Spec.From, tx.From)
	assert.Equal(t, scd.Spec.TxType, string(tx.Type))
	assert.Equal(t, scd.Spec.Bytecode, tx.Bytecode.String())
	// Additional assertions can be made on tx.ABI, tx.Data, etc.
	assert.NotNil(t, tx.ABI)
	assert.NotNil(t, tx.Data)
}

func TestBuildLinkReferences_Success(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			LinkedContracts: map[string]string{
				"LibName": "{{ .status.resolvedContractAddresses.LibName }}",
			},
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContactDependenciesStatus: corev1alpha1.ContactDependenciesStatus{
				ResolvedContractAddresses: map[string]string{
					"LibName": "0x3078616263646566313233343536373839300000",
				},
			},
		},
	}

	reconciler := &SmartContractDeploymentReconciler{}

	linkRefs, err := reconciler.buildLinkReferences(scd)
	require.NoError(t, err)
	require.NotNil(t, linkRefs)
	assert.Contains(t, linkRefs, "LibName")
	assert.Equal(t, "0x3078616263646566313233343536373839300000", linkRefs["LibName"].String())
}

func TestBuildLinkReferences_InvalidTemplate(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			LinkedContracts: map[string]string{
				"LibName": "{{ .invalidField }}",
			},
		},
	}

	reconciler := &SmartContractDeploymentReconciler{}

	_, err := reconciler.buildLinkReferences(scd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go template failed for linked contract")
}
func TestSmartContractDeploymentReconcilePaladin(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhaseReady,
		},
	}

	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Spec: corev1alpha1.SmartContractDeploymentSpec{
			Node: "test-node",
		},
	}

	reconciler, err := setupSmartContractDeploymentTestReconciler(paladin, scd)
	require.NoError(t, err)

	ctx := context.Background()

	reqs := reconciler.reconcilePaladin(ctx, paladin)
	require.Len(t, reqs, 1)
	assert.Equal(t, "test-scd", reqs[0].Name)
}

func TestReconcilePaladin_NotReady(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhasePending,
		},
	}

	reconciler, err := setupSmartContractDeploymentTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()

	reqs := reconciler.reconcilePaladin(ctx, paladin)
	require.Len(t, reqs, 0)
}
