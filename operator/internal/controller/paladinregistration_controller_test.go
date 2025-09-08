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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("PaladinRegistration Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		paladinregistration := &corev1alpha1.PaladinRegistration{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PaladinRegistration")
			err := k8sClient.Get(ctx, typeNamespacedName, paladinregistration)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.PaladinRegistration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.PaladinRegistrationSpec{
						Registry:          "evm-registry",
						RegistryAdminNode: "node1",
						RegistryAdminKey:  "deployKey",
						Node:              "node1",
						NodeKey:           "registryAdmin",
						Transports:        []string{"grpc"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &corev1alpha1.PaladinRegistration{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PaladinRegistration")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PaladinRegistrationReconciler{
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

func setupPaladinRegistrationTestReconciler(objs ...runtime.Object) (*PaladinRegistrationReconciler, error) {
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
		WithStatusSubresource(&corev1alpha1.PaladinRegistration{}).
		Build()

	reconciler := &PaladinRegistrationReconciler{
		Client: client,
		Scheme: scheme,
	}

	return reconciler, nil
}

func TestPaladinRegistrationReconcile_NewRegistration(t *testing.T) {

	// Create a PaladinRegistration resource
	reg := &corev1alpha1.PaladinRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registration",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrationSpec{
			Registry:          "test-registry",
			RegistryAdminNode: "admin-node",
			RegistryAdminKey:  "admin-key",
			Node:              "node1",
			NodeKey:           "node-key",
			Transports:        []string{"transport1", "transport2"},
		},
	}

	// Create a PaladinRegistry resource
	registry := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			ContractAddress: "0x0000000000000000000000000000000000000001",
		},
	}

	reconciler, err := setupPaladinRegistrationTestReconciler(reg, registry)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-registration",
			Namespace: "default",
		},
	}

	// Mock external dependencies (e.g., getPaladinRPC, transaction submission)
	// This requires creating interfaces or using dependency injection for the functions like getPaladinRPC.

	// For the purpose of this test, we will assume these dependencies are correctly mocked.

	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	// Since we are not implementing the full transaction submission and external calls,
	// we can't assert much more here without further mocking.

	// Fetch the updated registration
	updatedReg := &corev1alpha1.PaladinRegistration{}
	err = reconciler.Get(ctx, req.NamespacedName, updatedReg)
	require.NoError(t, err)
	// We can check if the status is updated as expected
	// For this, we'd need to simulate the transaction submission and status changes.
}

func TestUpdateRegistrationStatusAndRequeue(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registration",
			Namespace: "default",
		},
		Status: corev1alpha1.PaladinRegistrationStatus{
			PublishCount: 1,
		},
	}

	reconciler, err := setupPaladinRegistrationTestReconciler(reg)
	require.NoError(t, err)

	ctx := context.Background()

	// Update status
	reg.Status.PublishCount = 2
	result, err := reconciler.updateStatusAndRequeue(ctx, reg, reg.Status.PublishCount)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated registration
	updatedReg := &corev1alpha1.PaladinRegistration{}
	err = reconciler.Get(ctx, types.NamespacedName{
		Name:      "test-registration",
		Namespace: "default",
	}, updatedReg)
	require.NoError(t, err)
	assert.Equal(t, 2, updatedReg.Status.PublishCount)
}

func TestGetRegistryAddress(t *testing.T) {
	reg := &corev1alpha1.PaladinRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registration",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinRegistrationSpec{
			Registry: "test-registry",
		},
	}

	registry := &corev1alpha1.PaladinRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Status: corev1alpha1.PaladinRegistryStatus{
			ContractAddress: "0x0000000000000000000000000000000000000001",
		},
	}

	reconciler, err := setupPaladinRegistrationTestReconciler(registry)
	require.NoError(t, err)

	ctx := context.Background()

	address, err := reconciler.getRegistryAddress(ctx, reg)
	require.NoError(t, err)
	require.NotNil(t, address)
	assert.Equal(t, "0x0000000000000000000000000000000000000001", address.String())
}

type MockPaladinRPC struct {
	NodeName          string
	NodeAddress       string
	ResolveEthAddress string
}

func (m *MockPaladinRPC) CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) error {
	switch method {
	case "transport_nodeName":
		*result.(*string) = m.NodeName
	default:
		return fmt.Errorf("unknown method")
	}
	return nil
}

func (m *MockPaladinRPC) KeyManager() *MockKeyManager {
	return &MockKeyManager{
		EthAddress: m.ResolveEthAddress,
	}
}

type MockKeyManager struct {
	EthAddress string
}

func (m *MockKeyManager) ResolveEthAddress(ctx context.Context, key string) (string, error) {
	return m.EthAddress, nil
}

// func TestBuildRegistrationTX(t *testing.T) {
// 	reg := &corev1alpha1.PaladinRegistration{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "test-registration",
// 			Namespace: "default",
// 		},
// 		Spec: corev1alpha1.PaladinRegistrationSpec{
// 			Node:             "node1",
// 			RegistryAdminKey: "admin-key",
// 			NodeKey:          "node-key",
// 		},
// 	}

// 	registryAddr := &pldtypes.EthAddress{}
// 	registryAddr.UnmarshalJSON([]byte(`"0x0000000000000000000000000000000000000001"`))

// 	// paladinRPC := &MockPaladinRPC{
// 	// 	NodeName:          "TestNode",
// 	// 	ResolveEthAddress: "0xabcdef1234567890",
// 	// }

// 	reconciler, err := setupPaladinRegistrationTestReconciler()
// 	require.NoError(t, err)

// 	ctx := context.Background()

// 	ready, tx, err := reconciler.buildRegistrationTX(ctx, reg, registryAddr)
// 	require.NoError(t, err)
// 	require.True(t, ready)
// 	require.NotNil(t, tx)
// 	assert.Equal(t, reg.Spec.RegistryAdminKey, tx.From)
// 	assert.Equal(t, registryAddr, tx.To)
// 	// Additional assertions can be made on tx.Data, etc.
// }

// func TestBuildTransportTX(t *testing.T) {
// 	reg := &corev1alpha1.PaladinRegistration{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "test-registration",
// 			Namespace: "default",
// 		},
// 		Spec: corev1alpha1.PaladinRegistrationSpec{
// 			Node:    "node1",
// 			NodeKey: "node-key",
// 		},
// 	}

// 	registryAddr := &pldtypes.EthAddress{}
// 	registryAddr.UnmarshalJSON([]byte(`"0x1234567890abcdef"`))

// 	paladinRPC := &MockPaladinRPC{
// 		NodeName:          "TestNode",
// 		ResolveEthAddress: "0xabcdef1234567890",
// 	}

// 	// Mock transport details
// 	paladinRPC.CallRPC = func(ctx context.Context, result interface{}, method string, params ...interface{}) error {
// 		switch method {
// 		case "transport_localTransportDetails":
// 			*result.(*string) = "transport-details"
// 		case "transport_nodeName":
// 			*result.(*string) = paladinRPC.NodeName
// 		case "reg_queryEntries":
// 			*result.(*[]*registryEntry) = []*registryEntry{
// 				{
// 					ID:   "entry-id",
// 					Name: paladinRPC.NodeName,
// 				},
// 			}
// 		default:
// 			return fmt.Errorf("unknown method")
// 		}
// 		return nil
// 	}

// 	reconciler, err := setupPaladinRegistrationTestReconciler()
// 	require.NoError(t, err)

// 	ctx := context.Background()

// 	ready, tx, err := reconciler.buildTransportTX(ctx, reg, registryAddr, "transport1")
// 	require.NoError(t, err)
// 	assert.True(t, ready)
// 	require.NotNil(t, tx)
// 	assert.Equal(t, reg.Spec.NodeKey, tx.From)
// 	assert.Equal(t, registryAddr, tx.To)
// 	// Additional assertions can be made on tx.Data, etc.
// }

type PaladinRPC interface {
	CallRPC(ctx context.Context, result interface{}, method string, params ...interface{}) error
	KeyManager() KeyManager
}

type KeyManager interface {
	ResolveEthAddress(ctx context.Context, key string) (string, error)
}

// func TestPaladinRegistrationReconcile_FullFlow(t *testing.T) {
// 	// Set up logger for testing
// 	log.SetLogger(zap.New(zap.UseDevMode(true)))

// 	// Create resources
// 	reg := &corev1alpha1.PaladinRegistration{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "test-registration",
// 			Namespace: "default",
// 		},
// 		Spec: corev1alpha1.PaladinRegistrationSpec{
// 			Registry:          "test-registry",
// 			RegistryAdminNode: "admin-node",
// 			RegistryAdminKey:  "admin-key",
// 			Node:              "node1",
// 			NodeKey:           "node-key",
// 			Transports:        []string{"transport1"},
// 		},
// 		Status: corev1alpha1.PaladinRegistrationStatus{
// 			PublishTxs: make(map[string]corev1alpha1.TransactionSubmission),
// 		},
// 	}

// 	registry := &corev1alpha1.PaladinRegistry{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "test-registry",
// 			Namespace: "default",
// 		},
// 		Status: corev1alpha1.PaladinRegistryStatus{
// 			ContractAddress: "0x1234567890abcdef",
// 		},
// 	}

// 	reconciler, err := setupPaladinRegistrationTestReconciler(reg, registry)
// 	require.NoError(t, err)

// 	// Mock PaladinRPCFactory
// 	reconciler.PaladinRPCFactory = func(ctx context.Context, nodeName, namespace string) (PaladinRPC, error) {
// 		return &MockPaladinRPC{
// 			NodeName:          "TestNode",
// 			ResolveEthAddress: "0xabcdef1234567890",
// 			CallRPC: func(ctx context.Context, result interface{}, method string, params ...interface{}) error {
// 				switch method {
// 				case "transport_nodeName":
// 					*result.(*string) = "TestNode"
// 				case "transport_localTransportDetails":
// 					*result.(*string) = "transport-details"
// 				case "reg_queryEntries":
// 					*result.(*[]*registryEntry) = []*registryEntry{
// 						{
// 							ID:   "entry-id",
// 							Name: "TestNode",
// 						},
// 					}
// 				default:
// 					return errors.New("unknown method")
// 				}
// 				return nil
// 			},
// 		}, nil
// 	}

// 	ctx := context.Background()
// 	req := ctrl.Request{
// 		NamespacedName: types.NamespacedName{
// 			Name:      "test-registration",
// 			Namespace: "default",
// 		},
// 	}

// 	// Run the reconcile loop
// 	result, err := reconciler.Reconcile(ctx, req)
// 	require.NoError(t, err)
// 	// Since we are not actually submitting transactions, we can't verify the transaction status changes.

// 	// Fetch the updated registration
// 	updatedReg := &corev1alpha1.PaladinRegistration{}
// 	err = reconciler.Get(ctx, req.NamespacedName, updatedReg)
// 	require.NoError(t, err)

// 	// We can check if the PublishCount has been updated
// 	assert.Equal(t, 2, updatedReg.Status.PublishCount) // Assuming both registration and transport transactions are counted
// }
