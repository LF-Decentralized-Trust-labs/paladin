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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"testing"
	"time"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("PaladinDomain Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		paladindomain := &corev1alpha1.PaladinDomain{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PaladinDomain")
			err := k8sClient.Get(ctx, typeNamespacedName, paladindomain)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.PaladinDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.PaladinDomainSpec{
						Plugin: corev1alpha1.PluginConfig{
							Type: "c-shared",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.PaladinDomain{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PaladinDomain")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PaladinDomainReconciler{
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

func setupPaladinDomainTestReconciler(objs ...runtime.Object) (*PaladinDomainReconciler, error) {
	scheme := runtime.NewScheme()
	err := corev1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&corev1alpha1.PaladinDomain{}).
		Build()

	r := &PaladinDomainReconciler{
		Client: client,
		Scheme: scheme,
	}

	return r, nil
}
func TestPaladinDomainReconcile_NewResource(t *testing.T) {

	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			RegistryAddress: "0x1234567890abcdef",
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-domain",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated domain
	updatedDomain := &corev1alpha1.PaladinDomain{}
	err = r.Get(ctx, req.NamespacedName, updatedDomain)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.DomainStatusPending, updatedDomain.Status.Status)
}

func TestPaladinDomainReconcile_WithRegistryAddress(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			RegistryAddress: "0x1234567890abcdef",
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
		Status: corev1alpha1.PaladinDomainStatus{
			Status: "",
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-domain",
			Namespace: "default",
		},
	}

	// First reconcile: set status to Pending
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated domain
	updatedDomain := &corev1alpha1.PaladinDomain{}
	err = r.Get(ctx, req.NamespacedName, updatedDomain)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.DomainStatusPending, updatedDomain.Status.Status)

	// Second reconcile: set status to Available
	result, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated domain
	err = r.Get(ctx, req.NamespacedName, updatedDomain)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.DomainStatusAvailable, updatedDomain.Status.Status)
	assert.Equal(t, "0x1234567890abcdef", updatedDomain.Status.RegistryAddress)
}
func TestUpdateStatusAndRequeue(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Status: corev1alpha1.PaladinDomainStatus{
			Status: corev1alpha1.DomainStatusPending,
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain)
	require.NoError(t, err)

	ctx := context.Background()

	// Update status
	domain.Status.Status = corev1alpha1.DomainStatusAvailable
	result, err := r.updateStatusAndRequeue(ctx, domain)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated domain
	updatedDomain := &corev1alpha1.PaladinDomain{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      "test-domain",
		Namespace: "default",
	}, updatedDomain)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.DomainStatusAvailable, updatedDomain.Status.Status)
}
func TestTrackContractDeploymentAndRequeue_PendingDeployment(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			SmartContractDeployment: "test-scd",
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
		Status: corev1alpha1.PaladinDomainStatus{
			Status: corev1alpha1.DomainStatusPending,
		},
	}

	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "",
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := r.trackContractDeploymentAndRequeue(ctx, domain)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 1 * time.Second}, result)
}

func TestTrackContractDeploymentAndRequeue_SuccessfulDeployment(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			SmartContractDeployment: "test-scd",
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
		Status: corev1alpha1.PaladinDomainStatus{
			Status: corev1alpha1.DomainStatusPending,
		},
	}

	scd := &corev1alpha1.SmartContractDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scd",
			Namespace: "default",
		},
		Status: corev1alpha1.SmartContractDeploymentStatus{
			ContractAddress: "0xabcdef1234567890",
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain, scd)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := r.trackContractDeploymentAndRequeue(ctx, domain)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Fetch the updated domain
	updatedDomain := &corev1alpha1.PaladinDomain{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      "test-domain",
		Namespace: "default",
	}, updatedDomain)
	require.NoError(t, err)
	assert.Equal(t, corev1alpha1.DomainStatusAvailable, updatedDomain.Status.Status)
	assert.Equal(t, "0xabcdef1234567890", updatedDomain.Status.RegistryAddress)
}
func TestPaladinDomainReconcile_MissingFields(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-domain",
			Namespace: "default",
		},
	}

	// First reconcile: set status to Pending
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{Requeue: false, RequeueAfter: 50 * time.Millisecond}, result)

	// Second reconcile: should return error due to missing fields
	_, err = r.Reconcile(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing registryAddress or smartContractDeployment")
}
func TestPaladinDomainReconcile_DeletedResource(t *testing.T) {
	r, err := setupPaladinDomainTestReconciler()
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-domain",
			Namespace: "default",
		},
	}

	// Reconcile should return without error
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}
func TestTrackContractDeploymentAndRequeue_SCDNotFound(t *testing.T) {
	domain := &corev1alpha1.PaladinDomain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-domain",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinDomainSpec{
			SmartContractDeployment: "missing-scd",
			Plugin: corev1alpha1.PluginConfig{
				Type:    "c-shared",
				Library: "libexample",
			},
			ConfigJSON:   "{}",
			AllowSigning: true,
		},
		Status: corev1alpha1.PaladinDomainStatus{
			Status: corev1alpha1.DomainStatusPending,
		},
	}

	r, err := setupPaladinDomainTestReconciler(domain)
	require.NoError(t, err)

	ctx := context.Background()

	result, err := r.trackContractDeploymentAndRequeue(ctx, domain)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: 1 * time.Second}, result)
}
