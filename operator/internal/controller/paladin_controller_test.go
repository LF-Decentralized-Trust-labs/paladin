/*
Copyright 2025.

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/operator/pkg/config"
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
						SigningModules: []corev1alpha1.SigningModuleConfig{
							{
								Name: "sm",
								Plugin: corev1alpha1.PluginConfig{
									Type:    "c-shared",
									Library: "/app/signingmodules/libsm.so",
								},
								ConfigJSON: `{"url": "http://smendpoint"}`,
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
				Paladin: config.Template{
					Labels: map[string]string{
						"env":  "production",
						"tier": "backend",
					},
				},
			}
			controllerReconciler := &PaladinReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				RPCClientManager: NewRPCCache(),
				config:           cfg,
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
		Paladin: config.Template{
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
		"env":                        "production",
		"tier":                       "backend",
		"version":                    "v1",
		"app.kubernetes.io/instance": "test-node",
		"app.kubernetes.io/name":     "paladin-test-node",
		"app.kubernetes.io/part-of":  "paladin",
	}

	assert.Equal(t, expectedLabels, labels, "labels should match expected labels")
}

func TestGeneratePaladinAuthConfig(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1alpha1.Paladin
		secret   *corev1.Secret
		wantErr  bool
		expected *pldconf.PaladinConfig
	}{
		{
			name: "Valid AuthConfig with secret",
			node: &corev1alpha1.Paladin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "default",
				},
				Spec: corev1alpha1.PaladinSpec{
					BaseLedgerEndpoint: &corev1alpha1.BaseLedgerEndpoint{
						Type: corev1alpha1.EndpointTypeNetwork,
						Endpoint: &corev1alpha1.NetworkLedgerEndpoint{
							Auth: &corev1alpha1.Auth{
								Type:   corev1alpha1.AuthTypeSecret,
								Secret: &corev1alpha1.AuthSecret{Name: "test-secret"},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			},
			wantErr: false,
			expected: &pldconf.PaladinConfig{
				Blockchain: pldconf.EthClientConfig{
					HTTP: pldconf.HTTPClientConfig{
						Auth: pldconf.HTTPBasicAuthConfig{
							Username: "testuser",
							Password: "testpass",
						},
					},
					WS: pldconf.WSClientConfig{
						HTTPClientConfig: pldconf.HTTPClientConfig{
							Auth: pldconf.HTTPBasicAuthConfig{
								Username: "testuser",
								Password: "testpass",
							},
						},
					},
				},
			},
		},
		{
			name: "Valid AuthConfig with secretRef (deprecated)",
			node: &corev1alpha1.Paladin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "default",
				},
				Spec: corev1alpha1.PaladinSpec{
					BaseLedgerEndpoint: &corev1alpha1.BaseLedgerEndpoint{
						Type: corev1alpha1.EndpointTypeNetwork,
						Endpoint: &corev1alpha1.NetworkLedgerEndpoint{
							Auth: &corev1alpha1.Auth{
								Type:      corev1alpha1.AuthTypeSecret,
								SecretRef: &corev1alpha1.AuthSecret{Name: "test-secret"},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			},
			wantErr: false,
			expected: &pldconf.PaladinConfig{
				Blockchain: pldconf.EthClientConfig{
					HTTP: pldconf.HTTPClientConfig{
						Auth: pldconf.HTTPBasicAuthConfig{
							Username: "testuser",
							Password: "testpass",
						},
					},
					WS: pldconf.WSClientConfig{
						HTTPClientConfig: pldconf.HTTPClientConfig{
							Auth: pldconf.HTTPBasicAuthConfig{
								Username: "testuser",
								Password: "testpass",
							},
						},
					},
				},
			},
		},
		{
			name: "Secret not found",
			node: &corev1alpha1.Paladin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "default",
				},
				Spec: corev1alpha1.PaladinSpec{
					BaseLedgerEndpoint: &corev1alpha1.BaseLedgerEndpoint{
						Type: corev1alpha1.EndpointTypeNetwork,
						Endpoint: &corev1alpha1.NetworkLedgerEndpoint{
							JSONRPC: "https://besu.node",
							WS:      "wss://besu.mode",
							Auth: &corev1alpha1.Auth{
								Type:   corev1alpha1.AuthTypeSecret,
								Secret: &corev1alpha1.AuthSecret{Name: "test-secret"},
							},
						},
					},
				},
			},
			secret:  nil,
			wantErr: true,
		},
		{
			name: "Missing AuthSecret",
			node: &corev1alpha1.Paladin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "default",
				},
				Spec: corev1alpha1.PaladinSpec{
					BaseLedgerEndpoint: &corev1alpha1.BaseLedgerEndpoint{
						Type: corev1alpha1.EndpointTypeNetwork,
						Endpoint: &corev1alpha1.NetworkLedgerEndpoint{
							Auth: &corev1alpha1.Auth{
								Type: corev1alpha1.AuthTypeSecret,
							},
						},
					},
				},
			},
			secret:  nil,
			wantErr: true,
		},
		{
			name: "Secret with no data",
			node: &corev1alpha1.Paladin{
				Spec: corev1alpha1.PaladinSpec{
					BaseLedgerEndpoint: &corev1alpha1.BaseLedgerEndpoint{
						Type: corev1alpha1.EndpointTypeNetwork,
						Endpoint: &corev1alpha1.NetworkLedgerEndpoint{
							Auth: &corev1alpha1.Auth{
								Type:   corev1alpha1.AuthTypeSecret,
								Secret: &corev1alpha1.AuthSecret{Name: "empty-secret"},
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-secret",
					Namespace: "default",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client and populate it with the secret if provided
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = corev1alpha1.AddToScheme(scheme)
			ctx := context.TODO()

			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			if tt.secret != nil {
				err := client.Create(ctx, tt.secret)
				require.NoError(t, err)
			}

			reconciler := &PaladinReconciler{
				Client: client,
			}

			// Call the method under test
			pldConf := &pldconf.PaladinConfig{}

			err := reconciler.generatePaladinAuthConfig(ctx, tt.node, tt.node.Spec.BaseLedgerEndpoint.Endpoint.Auth, pldConf)

			// Verify the results
			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Did not expect an error but got one")
				assert.Equal(t, tt.expected.Blockchain.HTTP.Auth, pldConf.Blockchain.HTTP.Auth, "HTTP Auth mismatch")
				assert.Equal(t, tt.expected.Blockchain.WS.Auth, pldConf.Blockchain.WS.Auth, "WS Auth mismatch")
			}
		})
	}
}

func setupTestReconciler(objs ...client.Object) (*PaladinReconciler, client.Client, error) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}
	err = appsv1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}
	err = policyv1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}
	err = corev1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, nil, err
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&corev1alpha1.Paladin{}). // Enable status subresource
		Build()

	cfg := &config.Config{
		Paladin: config.Template{
			Image:           "paladin:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Envs:            map[string]string{},
			Labels:          map[string]string{},
		},
		Postgres: config.Template{
			Image:           "postgres:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Envs:            map[string]string{},
			Labels:          map[string]string{},
		},
	}

	reconciler := &PaladinReconciler{
		Client:           client,
		Scheme:           scheme,
		config:           cfg,
		RPCClientManager: NewRPCCache(),
		Changes:          NewInFlight(1 * time.Second),
	}

	return reconciler, client, nil
}
func TestPaladinReconcile_NewResource(t *testing.T) {
	// Set up logger for testing
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a Paladin resource with TypeMeta correctly set
	paladin := &corev1alpha1.Paladin{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.paladin.io/v1alpha1",
			Kind:       "Paladin",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinSpec{
			Database: corev1alpha1.Database{ // Ensure correct spec type
				Mode: corev1alpha1.DBMode_EmbeddedSQLite,
			},
		},
	}

	// Set up the test reconciler with the Paladin resource
	reconciler, client, err := setupTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      paladin.Name,
			Namespace: paladin.Namespace,
		},
	}

	// Invoke Reconcile first time
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Fetch the updated Paladin resource
	updatedPaladin := &corev1alpha1.Paladin{}
	err = client.Get(ctx, req.NamespacedName, updatedPaladin)
	require.NoError(t, err)

	// Check that the status phase is set to Pending
	assert.Equal(t, corev1alpha1.StatusPhaseReady, updatedPaladin.Status.Phase)

	// Simulate StatefulSet becoming ready
	statefulSet := &appsv1.StatefulSet{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      generatePaladinName(paladin.Name),
		Namespace: paladin.Namespace,
	}, statefulSet)
	require.NoError(t, err)

	// Manually update the StatefulSet's status to indicate readiness
	if statefulSet.Spec.Replicas != nil {
		replicas := *statefulSet.Spec.Replicas
		statefulSet.Status.Replicas = replicas
		statefulSet.Status.ReadyReplicas = replicas
	} else {
		// Default to 1 replica if not set
		statefulSet.Status.Replicas = 1
		statefulSet.Status.ReadyReplicas = 1
	}
	err = client.Status().Update(ctx, statefulSet)
	require.NoError(t, err)

	// Invoke Reconcile second time to update status to Ready
	result, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Fetch the updated Paladin resource again
	err = client.Get(ctx, req.NamespacedName, updatedPaladin)
	require.NoError(t, err)

	// Check that the status phase is set to Ready
	assert.Equal(t, corev1alpha1.StatusPhaseReady, updatedPaladin.Status.Phase)

	// delete the resource
	err = client.Delete(ctx, updatedPaladin)
	require.NoError(t, err)

	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	// Check that the resource is deleted
	err = client.Get(ctx, req.NamespacedName, updatedPaladin)
	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "Expected resource to be not found")
}

func TestPaladinCreateService(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinSpec{},
	}

	reconciler, client, err := setupTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()
	name := generatePaladinName(paladin.Name)

	svc, err := reconciler.createService(ctx, paladin, name)
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Fetch the service from the fake client
	fetchedSvc := &corev1.Service{}
	err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: paladin.Namespace}, fetchedSvc)
	require.NoError(t, err)

	// Verify the service properties
	assert.Equal(t, name, fetchedSvc.Name)
	assert.Equal(t, paladin.Namespace, fetchedSvc.Namespace)
	assert.Equal(t, corev1.ServiceTypeClusterIP, fetchedSvc.Spec.Type)
	assert.Len(t, fetchedSvc.Spec.Ports, 2)
}

func TestPaladinCreateConfigMap(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinSpec{
			Database: corev1alpha1.Database{
				Mode: corev1alpha1.DBMode_EmbeddedSQLite,
			},
		},
	}

	reconciler, client, err := setupTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()
	name := generatePaladinName(paladin.Name)

	configSum, tlsSecrets, cm, err := reconciler.createConfigMap(ctx, paladin, name)
	require.NoError(t, err)
	require.NotNil(t, cm)
	assert.NotEmpty(t, configSum)
	assert.Empty(t, tlsSecrets)

	// Fetch the ConfigMap from the fake client
	fetchedCM := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: paladin.Namespace}, fetchedCM)
	require.NoError(t, err)

	// Verify the ConfigMap data
	assert.Contains(t, fetchedCM.Data, "pldconf.paladin.yaml")
}

func TestPaladinCreateStatefulSet(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinSpec{
			Database: corev1alpha1.Database{
				Mode: corev1alpha1.DBMode_EmbeddedSQLite,
			},
		},
	}

	reconciler, client, err := setupTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()
	name := generatePaladinName(paladin.Name)
	configSum := "test-config-sum"

	ss, err := reconciler.createStatefulSet(ctx, paladin, name, nil, configSum)
	require.NoError(t, err)
	require.NotNil(t, ss)

	// Fetch the StatefulSet from the fake client
	fetchedSS := &appsv1.StatefulSet{}
	err = client.Get(ctx, types.NamespacedName{Name: name, Namespace: paladin.Namespace}, fetchedSS)
	require.NoError(t, err)

	// Verify the StatefulSet properties
	assert.Equal(t, name, fetchedSS.Name)
	assert.Equal(t, paladin.Namespace, fetchedSS.Namespace)
	assert.Len(t, fetchedSS.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "paladin", fetchedSS.Spec.Template.Spec.Containers[0].Name)
}

func TestGeneratePaladinConfig(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
		Spec: corev1alpha1.PaladinSpec{
			Database: corev1alpha1.Database{
				Mode: corev1alpha1.DBMode_EmbeddedSQLite,
			},
		},
	}

	reconciler, _, err := setupTestReconciler()
	require.NoError(t, err)

	ctx := context.Background()
	name := generatePaladinName(paladin.Name)

	configYAML, tlsSecrets, err := reconciler.generatePaladinConfig(ctx, paladin, name)
	require.NoError(t, err)
	require.NotEmpty(t, configYAML)
	assert.Empty(t, tlsSecrets)

	// Verify that the generated config contains expected values
	assert.Contains(t, configYAML, `nodeName: test-node`)
}
func TestGetPaladinURLEndpoint(t *testing.T) {
	paladin := &corev1alpha1.Paladin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-node",
			Namespace: "default",
		},
	}

	_, client, err := setupTestReconciler(paladin)
	require.NoError(t, err)

	ctx := context.Background()

	url, err := getPaladinURLEndpoint(ctx, client, paladin.Name, paladin.Namespace)
	require.NoError(t, err)
	assert.Equal(t, "http://paladin-test-node.default.svc.cluster.local:8548", url)
}

func TestGeneratePaladinName(t *testing.T) {
	name := generatePaladinName("test-node")
	assert.Equal(t, "paladin-test-node", name)
}
