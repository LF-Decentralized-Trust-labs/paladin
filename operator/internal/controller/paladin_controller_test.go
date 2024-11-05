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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/operator/pkg/config"
)

var _ = Describe("Paladin Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		node := &corev1alpha1.Paladin{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: "default",
			},
		}
		BeforeEach(func() {
			By("creating the custom resource for the Kind Node")
			err := k8sClient.Get(ctx, typeNamespacedName, node)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.Paladin{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: corev1alpha1.PaladinSpec{
						Database: corev1alpha1.Database{
							Mode:          "sidecarPostgres",
							MigrationMode: "auto",
						},
						BesuNode: "node1",
						SecretBackedSigners: []corev1alpha1.SecretBackedSigner{
							{
								Name:   "signer-1",
								Secret: "node1.keys",
								Type:   "autoHDWallet",
							},
						},
						Transports: []corev1alpha1.TransportConfig{
							{
								Name: "grpc",
								Plugin: corev1alpha1.PluginConfig{
									Type:    "c-shared",
									Library: "/app/transports/libgrpc.so",
								},
								Ports: []corev1.ServicePort{
									{
										Name:     "transport-grpc",
										Port:     9000,
										Protocol: corev1.ProtocolTCP,
									},
								},
								ConfigJSON: `{"port": 9000,"address": "0.0.0.0"}`,
							},
						},
						Domains: []corev1alpha1.DomainReference{
							{
								LabelReference: corev1alpha1.LabelReference{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{
											"paladin.io/domain-name": "noto",
										},
									},
								},
							},
						},
						Registries: []corev1alpha1.RegistryReference{
							{
								LabelReference: corev1alpha1.LabelReference{
									LabelSelector: metav1.LabelSelector{
										MatchLabels: map[string]string{
											"paladin.io/registry-name": "evm-registry",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.Paladin{}

			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Node")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			cfg := &config.Config{
				Paladin: struct {
					Image           string            `json:"image"`
					ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
					Labels          map[string]string `json:"labels"`
					Annotations     map[string]string `json:"annotations"`
					Envs            map[string]string `json:"envs"`
				}{
					Labels: map[string]string{
						"env":  "production",
						"tier": "backend",
					},
				},
			}
			controllerReconciler := &PaladinReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				config: cfg,
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

func TestPaladin_GetLabels(t *testing.T) {
	// Mock configuration
	config := config.Config{
		Paladin: struct {
			Image           string            `json:"image"`
			ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
			Labels          map[string]string `json:"labels"`
			Annotations     map[string]string `json:"annotations"`
			Envs            map[string]string `json:"envs"`
		}{
			Labels: map[string]string{
				"env":  "production",
				"tier": "backend",
			},
		},
	}

	// Initialize PaladinReconciler with mock config
	r := &PaladinReconciler{}
	r.config = &config

	// Mock Paladin node
	node := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	// Extra labels
	extraLabels := map[string]string{
		"version": "v1",
	}

	// Call getLabels
	labels := r.getLabels(node, extraLabels)

	// Assertions
	expectedLabels := map[string]string{
		"app":     "paladin-test-node",
		"env":     "production",
		"tier":    "backend",
		"version": "v1",
	}

	assert.Equal(t, expectedLabels, labels, "labels should match expected labels")
}
