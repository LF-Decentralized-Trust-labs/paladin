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
)

var _ = Describe("TransactionInvoke Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		transactioninvoke := &corev1alpha1.TransactionInvoke{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind TransactionInvoke")
			err := k8sClient.Get(ctx, typeNamespacedName, transactioninvoke)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.TransactionInvoke{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "core.paladin.io/v1alpha1",
						Kind:       "TransactionInvoke",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.TransactionInvokeSpec{
						TxType: "public",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.TransactionInvoke{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance TransactionInvoke")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &TransactionInvokeReconciler{
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

func setupTransactionInvokeTestReconciler(objs ...runtime.Object) (*TransactionInvokeReconciler, client.Client, error) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}
	err = corev1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&corev1alpha1.TransactionInvoke{}).
		WithStatusSubresource(&corev1alpha1.Paladin{}).
		WithStatusSubresource(&corev1alpha1.SmartContractDeployment{}).
		Build()

	reconciler := &TransactionInvokeReconciler{
		Client: client,
		Scheme: scheme,
	}

	return reconciler, client, nil
}
func TestTransactionInvokeReconcile_Success(t *testing.T) {
	// Set up logger for testing
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a TransactionInvoke resource
	txi := &corev1alpha1.TransactionInvoke{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-txi",
			Namespace: "default",
		},
		Spec: corev1alpha1.TransactionInvokeSpec{
			Node:                   "test-node",
			TxType:                 "public",
			From:                   "0x123",
			ToTemplate:             "{{ .status.resolvedContractAddresses.MyContract }}",
			ParamsJSONTemplate:     `{"method": "myMethod", "params": []}`,
			ABIJSON:                `[{"name": "myMethod", "type": "function", "inputs": [], "outputs": []}]`,
			ContractDeploymentDeps: []string{"my-contract-deployment"},
		},
	}

	reconciler, _, err := setupTransactionInvokeTestReconciler(txi)
	require.NoError(t, err)

	// Inject mocked dependencies
	reconciler.checkDepsFunc = func(ctx context.Context, c client.Client, namespace string, requiredDeployments []string, status *corev1alpha1.ContactDependenciesStatus) (bool, bool, error) {
		status.ResolvedContractAddresses = map[string]string{
			"MyContract": "0xabcdef1234567890",
		}
		status.ContractDepsSummary = "1/1"
		return false, true, nil
	}

	// Mock transaction reconcile
	reconciler.newTransactionReconcileFunc = func(c client.Client, r *rpcClientManager, idempotencyKeyPrefix string, nodeName string, namespace string, pStatus *corev1alpha1.TransactionSubmission, timeout string, txFactory func() (bool, *pldapi.TransactionInput, error)) transactionReconcileInterface {
		return &mockTransactionReconcile{
			pStatus:           pStatus,
			statusChangedFlag: true,
			succeededFlag:     true,
		}
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-txi",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated TransactionInvoke
	updatedTxi := &corev1alpha1.TransactionInvoke{}
	err = reconciler.Get(ctx, req.NamespacedName, updatedTxi)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.TransactionStatusSuccess, updatedTxi.Status.TransactionStatus)
}

func TestTransactionInvokeBuildDeployTransaction_Success(t *testing.T) {
	txi := &corev1alpha1.TransactionInvoke{
		Spec: corev1alpha1.TransactionInvokeSpec{
			TxType:             "public",
			From:               "0x123",
			ToTemplate:         "{{ .status.resolvedContractAddresses.MyContract }}",
			ParamsJSONTemplate: `{"method": "myMethod", "params": []}`,
			ABIJSON:            `[{"name": "myMethod", "type": "function", "inputs": [], "outputs": []}]`,
		},
		Status: corev1alpha1.TransactionInvokeStatus{
			ContactDependenciesStatus: corev1alpha1.ContactDependenciesStatus{
				ResolvedContractAddresses: map[string]string{
					"MyContract": "0x3078616263646566313233343536373839300000",
				},
			},
		},
	}

	reconciler := &TransactionInvokeReconciler{}

	ready, txInput, err := reconciler.buildDeployTransaction(txi)
	require.NoError(t, err)
	assert.True(t, ready)
	assert.NotNil(t, txInput)
	assert.Equal(t, "0x3078616263646566313233343536373839300000", txInput.To.String())
	assert.Equal(t, txi.Spec.From, txInput.From)
	assert.Equal(t, txi.Spec.TxType, string(txInput.Type))
}

func TestBuildDeployTransaction_InvalidToTemplate(t *testing.T) {
	txi := &corev1alpha1.TransactionInvoke{
		Spec: corev1alpha1.TransactionInvokeSpec{
			ToTemplate: "{{ .invalidField }}",
		},
	}

	reconciler := &TransactionInvokeReconciler{}

	_, _, err := reconciler.buildDeployTransaction(txi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toTemplate failed")
}

func TestCheckSmartContractDeps_Resolved(t *testing.T) {
	status := &corev1alpha1.ContactDependenciesStatus{}
	scheme := runtime.NewScheme()
	err := corev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep1",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0xabcdef1234567890",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(scd).
		Build()

	resolved, ready, err := checkSmartContractDeps(context.Background(), client, "default", []string{"dep1"}, status)
	require.NoError(t, err)
	assert.True(t, resolved)
	assert.False(t, ready)
	assert.Equal(t, "0xabcdef1234567890", status.ResolvedContractAddresses["dep1"])
}

func TestCheckSmartContractDeps_ResolveDependency(t *testing.T) {
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep1",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0x3078616263646566313233343536373839300000",
		},
	}

	status := &corev1alpha1.ContactDependenciesStatus{}
	_, client, _ := setupTransactionInvokeTestReconciler(scd)

	resolved, ready, err := checkSmartContractDeps(context.Background(), client, "default", []string{"dep1"}, status)
	require.NoError(t, err)
	assert.True(t, resolved)
	assert.False(t, ready)
	assert.Equal(t, "0x3078616263646566313233343536373839300000", status.ResolvedContractAddresses["dep1"])
}

func TestTransactionInvokeReconcilePaladin(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Status: corev1alpha1.Status{
			Phase: corev1alpha1.StatusPhaseReady,
		},
	}

	txi := &corev1alpha1.TransactionInvoke{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-txi",
			Namespace: "default",
		},
		Spec: corev1alpha1.TransactionInvokeSpec{
			Node: "test-node",
		},
	}

	reconciler, _, err := setupTransactionInvokeTestReconciler(paladin, txi)
	require.NoError(t, err)

	ctx := context.Background()

	reqs := reconciler.reconcilePaladin(ctx, paladin)
	require.Len(t, reqs, 1)
	assert.Equal(t, "test-txi", reqs[0].Name)
}
