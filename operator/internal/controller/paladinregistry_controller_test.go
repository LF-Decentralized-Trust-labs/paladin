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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("PaladinRegistry Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		paladinregistry := &corev1alpha1.PaladinRegistry{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PaladinRegistry")
			err := k8sClient.Get(ctx, typeNamespacedName, paladinregistry)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.PaladinRegistry{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.PaladinRegistrySpec{
						Type: corev1alpha1.RegistryTypeEVM,
						EVM: corev1alpha1.EVMRegistryConfig{
							SmartContractDeployment: "registry",
						},
						Plugin: corev1alpha1.PluginConfig{
							Type:    "c-shared",
							Library: "/app/registries/libevm.so",
						},
						ConfigJSON: "{}",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.PaladinRegistry{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PaladinRegistry")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PaladinRegistryReconciler{
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

func setupPaladinRegistryTestReconciler(objs ...runtime.Object) (*PaladinRegistryReconciler, error) {
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
		WithStatusSubresource(&corev1alpha1.PaladinRegistry{}).
		Build()

	reconciler := &PaladinRegistryReconciler{
		Client: client,
		Scheme: scheme,
	}

	return reconciler, nil
}

func TestPaladinRegistryReconcile_WithContractAddress(t *testing.T) {
	// Set up logger for testing
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a PaladinRegistry resource with a fixed contract address
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				ContractAddress: "0x1234567890abcdef",
			},
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	// Run the reconcile function
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	updatedReg := &corev1alpha1.PaladinRegistry{}
	err = reconciler.Get(ctx, req.NamespacedName, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusAvailable, updatedReg.Status.Status)
	assert.Equal(t, "0x1234567890abcdef", updatedReg.Status.ContractAddress)
}

func TestPaladinRegistryReconcile_WithSmartContractDeployment(t *testing.T) {
	// Create a PaladinRegistry resource with a SmartContractDeployment reference
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
	}

	// Create a SmartContractDeployment resource with a ContractAddress
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0xabcdef1234567890",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg, scd)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	// First reconcile: status should be set to Pending
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	updatedReg := &corev1alpha1.PaladinRegistry{}
	err = reconciler.Get(ctx, req.NamespacedName, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusPending, updatedReg.Status.Status)

	// Second reconcile: should update status to Available
	result, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	err = reconciler.Get(ctx, req.NamespacedName, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusAvailable, updatedReg.Status.Status)
	assert.Equal(t, "0xabcdef1234567890", updatedReg.Status.ContractAddress)
}

func TestPaladinRegistryReconcile_MissingContractInfo(t *testing.T) {
	// Create a PaladinRegistry resource without ContractAddress or SmartContractDeployment
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	// Run the reconcile function
	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing contractAddress or smartContractDeployment")
	assert.Equal(t, ctrl.Result{}, result)
}
func TestPaladinRegistryReconcile_ResourceNotFound(t *testing.T) {
	reconciler, err := setupPaladinRegistryTestReconciler()
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-registry",
			Namespace: "default",
		},
	}

	// Reconcile should return without error
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestTrackContractDeploymentAndRequeue_DeploymentNotFound(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status: corev1alpha1.RegistryStatusPending,
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := reconciler.trackContractDeploymentAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 1 * time.Second}, result)
}

func TestPaladinRegistryTrackContractDeploymentAndRequeue_PendingDeployment(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status: corev1alpha1.RegistryStatusPending,
		},
	}

	// Create a SmartContractDeployment resource without a ContractAddress
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := reconciler.trackContractDeploymentAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 1 * time.Second}, result)
}

func TestPaladinRegistryTrackContractDeploymentAndRequeue_SuccessfulDeployment(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status: corev1alpha1.RegistryStatusPending,
		},
	}

	// Create a SmartContractDeployment resource with a ContractAddress
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0xabcdef1234567890",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := reconciler.trackContractDeploymentAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	updatedReg := &corev1alpha1.PaladinRegistry{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      reg.Name,
		Namespace: reg.Namespace,
	}, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusAvailable, updatedReg.Status.Status)
	assert.Equal(t, "0xabcdef1234567890", updatedReg.Status.ContractAddress)
}

func TestPaladinRegistryReconcile_GetError(t *testing.T) {
	reconciler, err := setupPaladinRegistryTestReconciler()
	require.NoError(t, err)

	// Mock client to return an error on Get
	reconciler.Client = &errorClient{
		Client: reconciler.Client,
		getErr: fmt.Errorf("failed to get resource"),
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get resource")
	assert.Equal(t, ctrl.Result{}, result)
}

type errorClient struct {
	client.Client
	getErr error
}

func (e *errorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if e.getErr != nil {
		return e.getErr
	}
	return e.Client.Get(ctx, key, obj, opts...)
}

func TestPaladinRegistryReconcile_DeletedResource(t *testing.T) {
	reconciler, err := setupPaladinRegistryTestReconciler()
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-registry",
			Namespace: "default",
		},
	}

	// Reconcile should return without error
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestUpdatePaladinRegistryStatusAndRequeue(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status:          corev1alpha1.RegistryStatusPending,
			ContractAddress: "",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()

	// Update status
	reg.Status.Status = corev1alpha1.RegistryStatusAvailable
	reg.Status.ContractAddress = "0xabcdef1234567890"
	result, err := reconciler.updateStatusAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	updatedReg := &corev1alpha1.PaladinRegistry{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      "test-registry",
		Namespace: "default",
	}, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusAvailable, updatedReg.Status.Status)
	assert.Equal(t, "0xabcdef1234567890", updatedReg.Status.ContractAddress)
}

func TestPaladinRegistryTrackContractDeploymentAndRequeuePendingDeployment(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status: corev1alpha1.RegistryStatusPending,
		},
	}

	// Create a SmartContractDeployment resource without a ContractAddress
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := reconciler.trackContractDeploymentAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 1 * time.Second}, result)
}

func TestPaladinRegistryTrackContractDeploymentAndRequeueSuccessfulDeployment(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: corev1alpha1.RegistryTypeEVM,
			EVM: corev1alpha1.EVMRegistryConfig{
				SmartContractDeployment: "test-scd",
			},
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			Status: corev1alpha1.RegistryStatusPending,
		},
	}

	// Create a SmartContractDeployment resource with a ContractAddress
	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0xabcdef1234567890",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := reconciler.trackContractDeploymentAndRequeue(ctx, reg)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registry
	updatedReg := &corev1alpha1.PaladinRegistry{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      "test-registry",
		Namespace: "default",
	}, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.RegistryStatusAvailable, updatedReg.Status.Status)
	assert.Equal(t, "0xabcdef1234567890", updatedReg.Status.ContractAddress)
}
func TestPaladinRegistryReconcile_NonEVMType(t *testing.T) {
	// Create a PaladinRegistry resource with a non-EVM type (assuming other types may be added in the future)
	reg := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrySpec{
			Type: "non-evm",
		},
	}

	reconciler, err := setupPaladinRegistryTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	// Run the reconcile function
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
