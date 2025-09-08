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

	// . "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"encoding/json"
	"testing"

	"github.com/LF-Decentralized-Trust-labs/paladin/testinfra/pkg/besugenesis"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// var _ = Describe("BesuGenesis Controller", func() {
// 	var (
// 		ctx                   context.Context
// 		resourceName          string
// 		namespace             string
// 		typeNamespacedName    types.NamespacedName
// 		besuGenesis           *corev1alpha1.BesuGenesis
// 		besuGenesisReconciler *BesuGenesisReconciler
// 	)

// 	BeforeEach(func() {
// 		ctx = context.Background()
// 		resourceName = "testnet"
// 		namespace = "default"
// 		typeNamespacedName = types.NamespacedName{
// 			Name:      resourceName,
// 			Namespace: namespace,
// 		}

// 		// Initialize BesuGenesis resource
// 		besuGenesis = &corev1alpha1.BesuGenesis{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      resourceName,
// 				Namespace: namespace,
// 			},
// 			Spec: corev1alpha1.BesuGenesisSpec{
// 				ChainID:           1337,
// 				GasLimit:          700000000,
// 				Consensus:         "qbft",
// 				BlockPeriod:       "100ms",
// 				EmptyBlockPeriod:  "10s",
// 				InitialValidators: []string{"validator1"},
// 			},
// 		}

// 		// Create the BesuGenesis resource
// 		By("Creating the BesuGenesis custom resource")
// 		err := k8sClient.Create(ctx, besuGenesis)
// 		Expect(err).NotTo(HaveOccurred())

// 		// Initialize BesuGenesisReconciler
// 		besuGenesisReconciler = &BesuGenesisReconciler{
// 			Client: k8sClient,
// 			Scheme: k8sClient.Scheme(),
// 		}
// 	})

// 	AfterEach(func() {
// 		// Delete the BesuGenesis resource
// 		By("Deleting the BesuGenesis custom resource")
// 		err := k8sClient.Delete(ctx, besuGenesis)
// 		Expect(err).NotTo(HaveOccurred())
// 	})

// 	Context("When reconciling a new BesuGenesis resource", func() {
// 		It("Should create the genesis ConfigMap when validator secrets are available", func() {
// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Checking that the genesis ConfigMap is created")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(err).NotTo(HaveOccurred())
// 			Expect(configMap.Data).To(HaveKey("genesis.json"))
// 		})

// 		It("Should wait for validator secrets if they are not available", func() {
// 			By("Reconciling the BesuGenesis resource without validator secrets")
// 			result, err := besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

// 			By("Ensuring the genesis ConfigMap is not created")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(errors.IsNotFound(err)).To(BeTrue())
// 		})

// 		It("Should set status to Ready when genesis ConfigMap is created", func() {
// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Verifying that the status is set to Ready")
// 			updatedGenesis := &corev1alpha1.BesuGenesis{}
// 			err = k8sClient.Get(ctx, typeNamespacedName, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())
// 			Expect(updatedGenesis.Status.Phase).To(Equal(corev1alpha1.StatusPhaseReady))
// 		})
// 	})

// 	Context("When updating the BesuGenesis resource", func() {
// 		It("Should not modify the genesis ConfigMap after creation", func() {
// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource to create the ConfigMap")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Updating the BesuGenesis resource")
// 			updatedGenesis := &corev1alpha1.BesuGenesis{}
// 			err = k8sClient.Get(ctx, typeNamespacedName, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())
// 			updatedGenesis.Spec.ChainID = 9999
// 			err = k8sClient.Update(ctx, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource after update")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Ensuring the genesis ConfigMap is not modified")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(err).NotTo(HaveOccurred())

// 			// Optionally, check that the ChainID is still the old value
// 			Expect(configMap.Data).To(HaveKey("genesis.json"))
// 			Expect(configMap.Data["genesis.json"]).To(ContainSubstring(`"chainId":1337`))
// 		})
// 	})

// 	Context("When deleting a BesuGenesis resource", func() {
// 		It("Should not delete the genesis ConfigMap", func() {
// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource to create the ConfigMap")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Deleting the BesuGenesis resource")
// 			err = k8sClient.Delete(ctx, besuGenesis)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Ensuring the genesis ConfigMap still exists")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(err).NotTo(HaveOccurred())
// 		})
// 	})

// 	Context("When validator secrets are missing", func() {
// 		It("Should not create the genesis ConfigMap and requeue", func() {
// 			By("Reconciling the BesuGenesis resource without validator secrets")
// 			result, err := besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())
// 			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

// 			By("Ensuring the genesis ConfigMap is not created")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(errors.IsNotFound(err)).To(BeTrue())
// 		})
// 	})

// 	Context("When there is an invalid BlockPeriod", func() {
// 		It("Should fail reconciliation with an error", func() {
// 			By("Updating the BesuGenesis resource with an invalid BlockPeriod")
// 			updatedGenesis := &corev1alpha1.BesuGenesis{}
// 			err := k8sClient.Get(ctx, typeNamespacedName, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())
// 			updatedGenesis.Spec.BlockPeriod = "invalid-duration"
// 			err = k8sClient.Update(ctx, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).To(HaveOccurred())
// 			Expect(err.Error()).To(ContainSubstring("invalid blockPeriod"))
// 		})
// 	})

// 	Context("When InitialValidators is empty", func() {
// 		It("Should fail reconciliation with an error", func() {
// 			By("Updating the BesuGenesis resource with empty InitialValidators")
// 			updatedGenesis := &corev1alpha1.BesuGenesis{}
// 			err := k8sClient.Get(ctx, typeNamespacedName, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())
// 			updatedGenesis.Spec.InitialValidators = []string{}
// 			err = k8sClient.Update(ctx, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).To(HaveOccurred())
// 			Expect(err.Error()).To(ContainSubstring("at least one initial validator must be provided"))
// 		})
// 	})

// 	Context("When validator secret has invalid address", func() {
// 		It("Should fail reconciliation with an error", func() {
// 			By("Creating the validator secret with invalid address")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("invalid-address"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).To(HaveOccurred())
// 			Expect(err.Error()).To(ContainSubstring("invalid address in identity secret"))
// 		})
// 	})

// 	Context("When Base genesis JSON is invalid", func() {
// 		It("Should fail reconciliation with an error", func() {
// 			By("Updating the BesuGenesis resource with invalid base JSON")
// 			updatedGenesis := &corev1alpha1.BesuGenesis{}
// 			err := k8sClient.Get(ctx, typeNamespacedName, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())
// 			updatedGenesis.Spec.Base = "invalid-json"
// 			err = k8sClient.Update(ctx, updatedGenesis)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err = k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).To(HaveOccurred())
// 			Expect(err.Error()).To(ContainSubstring("supplied base genesis JSON could not be parsed"))
// 		})
// 	})

// 	Context("When BlockPeriod is less than 1s", func() {
// 		It("Should set BlockPeriodMilliseconds in QBFT config", func() {
// 			By("Creating the validator secret")
// 			validatorSecret := &corev1.Secret{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "validator1-secret",
// 					Namespace: namespace,
// 					Labels: map[string]string{
// 						"besu-node-id": "validator1",
// 					},
// 				},
// 				Data: map[string][]byte{
// 					"address": []byte("0x0000000000000000000000000000000000000001"),
// 				},
// 			}
// 			err := k8sClient.Create(ctx, validatorSecret)
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Reconciling the BesuGenesis resource")
// 			_, err = besuGenesisReconciler.Reconcile(ctx, reconcile.Request{
// 				NamespacedName: typeNamespacedName,
// 			})
// 			Expect(err).NotTo(HaveOccurred())

// 			By("Verifying the genesis ConfigMap contains BlockPeriodMilliseconds")
// 			configMap := &corev1.ConfigMap{}
// 			err = k8sClient.Get(ctx, types.NamespacedName{
// 				Name:      generateBesuGenesisName(resourceName),
// 				Namespace: namespace,
// 			}, configMap)
// 			Expect(err).NotTo(HaveOccurred())
// 			Expect(configMap.Data).To(HaveKey("genesis.json"))
// 			Expect(configMap.Data["genesis.json"]).To(ContainSubstring(`"blockperiodmilliseconds":100`))
// 		})
// 	})
// })

func setupGenesisTestReconciler(objs ...runtime.Object) (*BesuGenesisReconciler, client.Client, error) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = corev1alpha1.AddToScheme(scheme)

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	r := &BesuGenesisReconciler{
		Client: client,
		Scheme: scheme,
	}

	return r, client, nil
}

func TestBesuGenesisReconcile(t *testing.T) {

	genesis := &corev1alpha1.BesuGenesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: corev1alpha1.BesuGenesisSpec{
			ChainID:           1337,
			GasLimit:          8000000,
			Consensus:         "qbft",
			BlockPeriod:       "5s",
			EmptyBlockPeriod:  "10s",
			InitialValidators: []string{"validator1"},
		},
	}

	// Create the initial validator secret
	validatorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator1-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator1",
			},
		},
		Data: map[string][]byte{
			"address": []byte("0x0000000000000000000000000000000000000001"),
		},
	}

	r, client, err := setupGenesisTestReconciler(genesis, validatorSecret)
	require.NoError(t, err)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-genesis",
			Namespace: "default",
		},
	}

	res, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)

	// Check if the ConfigMap was created
	configMap := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      generateBesuGenesisName("test-genesis"),
		Namespace: "default",
	}, configMap)
	require.NoError(t, err)
	assert.Contains(t, configMap.Data, "genesis.json")
}

func TestCreateConfigMap(t *testing.T) {

	genesis := &corev1alpha1.BesuGenesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: corev1alpha1.BesuGenesisSpec{
			ChainID:          1337,
			GasLimit:         8000000,
			Consensus:        "qbft",
			BlockPeriod:      "5s",
			EmptyBlockPeriod: "10s",
			InitialValidators: []string{
				"validator1",
			},
		},
	}

	// Create the initial validator secret
	validatorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator1-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator1",
			},
		},
		Data: map[string][]byte{
			"address": []byte("0x0000000000000000000000000000000000000001"),
		},
	}

	r, _, err := setupGenesisTestReconciler(genesis, validatorSecret)
	require.NoError(t, err)

	// err = client.Create(context.Background(), validatorSecret)
	// require.NoError(t, err)

	ctx := context.Background()
	configMap, ready, err := r.createConfigMap(ctx, genesis)
	require.NoError(t, err)
	require.True(t, ready)
	require.NotNil(t, configMap)
	assert.Equal(t, generateBesuGenesisName(genesis.Name), configMap.Name)
}

func TestGenerateNewGenesisMap(t *testing.T) {
	genesis := &corev1alpha1.BesuGenesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: corev1alpha1.BesuGenesisSpec{
			Base:             "",
			ChainID:          1337,
			GasLimit:         8000000,
			Consensus:        "qbft",
			BlockPeriod:      "5s",
			EmptyBlockPeriod: "10s",
			InitialValidators: []string{
				"validator1",
			},
		},
	}

	// Mock the validator secret
	validatorSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator1-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator1",
			},
		},
		Data: map[string][]byte{
			"address": []byte("0x0000000000000000000000000000000000000001"),
		},
	}

	r, _, err := setupGenesisTestReconciler(validatorSecret)
	require.NoError(t, err)

	ctx := context.Background()
	name := generateBesuGenesisName(genesis.Name)
	configMap, ready, err := r.generateNewGenesisMap(ctx, genesis, name)
	require.NoError(t, err)
	assert.True(t, ready)
	require.NotNil(t, configMap)
	assert.Equal(t, name, configMap.Name)
	assert.Contains(t, configMap.Data, "genesis.json")

	// Optionally, validate the content of the genesis.json
	var genesisData map[string]interface{}
	err = json.Unmarshal([]byte(configMap.Data["genesis.json"]), &genesisData)
	require.NoError(t, err)
	assert.Equal(t, float64(1337), genesisData["config"].(map[string]interface{})["chainId"])
}

func TestLoadInitialValidatorIDSecrets(t *testing.T) {
	validatorSecret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator1-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator1",
			},
		},
	}

	validatorSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator2-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator2",
			},
		},
	}

	r, _, err := setupGenesisTestReconciler(validatorSecret1, validatorSecret2)
	require.NoError(t, err)

	ctx := context.Background()
	secrets, err := r.loadInitialValidatorIDSecrets(ctx, "default", []string{"validator1", "validator2"})
	require.NoError(t, err)
	assert.Len(t, secrets, 2)
}

func TestGetInitialValidators(t *testing.T) {
	genesis := &corev1alpha1.BesuGenesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: corev1alpha1.BesuGenesisSpec{
			InitialValidators: []string{"validator1", "validator2"},
		},
	}

	validatorSecret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator1-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator1",
			},
		},
		Data: map[string][]byte{
			"address": []byte("0x0000000000000000000000000000000000000001"),
		},
	}

	validatorSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "validator2-secret",
			Namespace: "default",
			Labels: map[string]string{
				"besu-node-id": "validator2",
			},
		},
		Data: map[string][]byte{
			"address": []byte("0x0000000000000000000000000000000000000002"),
		},
	}

	r, _, err := setupGenesisTestReconciler(validatorSecret1, validatorSecret2)
	require.NoError(t, err)

	ctx := context.Background()
	addresses, ready, err := r.getInitialValidators(ctx, genesis)
	require.NoError(t, err)
	assert.True(t, ready)
	assert.Len(t, addresses, 2)
	assert.Equal(t, "0x0000000000000000000000000000000000000001", addresses[0].String())
	assert.Equal(t, "0x0000000000000000000000000000000000000002", addresses[1].String())
}
func TestSetQBFTConfig(t *testing.T) {
	validatorAddresses := make([]ethtypes.Address0xHex, 2)
	validatorAddresses[0].SetString("0x0000000000000000000000000000000000000001")
	validatorAddresses[1].SetString("0x0000000000000000000000000000000000000002")

	genesis := &corev1alpha1.BesuGenesis{
		Spec: corev1alpha1.BesuGenesisSpec{
			BlockPeriod:      "5s",
			EmptyBlockPeriod: "10s",
		},
	}

	g := &besugenesis.GenesisJSON{
		Config: besugenesis.GenesisConfig{
			QBFT: &besugenesis.QBFTConfig{},
		},
	}

	r := &BesuGenesisReconciler{}
	err := r.setQBFTConfig(validatorAddresses, genesis, g)
	require.NoError(t, err)
	assert.Equal(t, 5, *g.Config.QBFT.BlockPeriodSeconds)
	assert.Equal(t, 10, *g.Config.QBFT.EmptyBlockPeriodSeconds)
}
func TestWrapGenesisInConfigMap(t *testing.T) {
	genesis := &corev1alpha1.BesuGenesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
	}

	genesisJSON := "{}"
	name := generateBesuGenesisName(genesis.Name)

	r := &BesuGenesisReconciler{}
	configMap := r.wrapGenesisInConfigMap(genesis, genesisJSON, name)

	assert.Equal(t, name, configMap.Name)
	assert.Equal(t, genesis.Namespace, configMap.Namespace)
	assert.Contains(t, configMap.Data, "genesis.json")
	assert.Equal(t, genesisJSON, configMap.Data["genesis.json"])
}
func TestGenerateBesuGenesisName(t *testing.T) {
	name := generateBesuGenesisName("test-genesis")
	assert.Equal(t, "besu-test-genesis-genesis", name)
}
func TestNearestIntegerAboveZero(t *testing.T) {
	assert.Equal(t, 1, nearestIntegerAboveZero(0.1))
	assert.Equal(t, 2, nearestIntegerAboveZero(1.5))
	assert.Equal(t, 5, nearestIntegerAboveZero(4.8))
	assert.Equal(t, 10, nearestIntegerAboveZero(10.0))
}
func TestPtrTo(t *testing.T) {
	i := 10
	pi := ptrTo(i)
	assert.Equal(t, &i, pi)
	assert.Equal(t, 10, *pi)

	s := "test"
	ps := ptrTo(s)
	assert.Equal(t, &s, ps)
	assert.Equal(t, "test", *ps)
}
