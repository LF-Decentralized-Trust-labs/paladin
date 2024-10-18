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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
	"github.com/tyler-smith/go-bip39"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	corev1alpha1 "github.com/kaleido-io/paladin/operator/api/v1alpha1"
	"github.com/kaleido-io/paladin/operator/pkg/config"
)

// PaladinReconciler reconciles a Paladin object
type PaladinReconciler struct {
	client.Client
	config *config.Config
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var PaladinCRMap = CRMap[corev1alpha1.Paladin, *corev1alpha1.Paladin, *corev1alpha1.PaladinList]{
	NewList:  func() *corev1alpha1.PaladinList { return new(corev1alpha1.PaladinList) },
	ItemsFor: func(list *corev1alpha1.PaladinList) []corev1alpha1.Paladin { return list.Items },
	AsObject: func(item *corev1alpha1.Paladin) *corev1alpha1.Paladin { return item },
}

// Reconcile implements the logic when a Paladin resource is created, updated, or deleted
func (r *PaladinReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Generate a name for the Paladin resources
	name := generatePaladinName(req.Name)

	// Fetch the Paladin instance
	var node corev1alpha1.Paladin
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if errors.IsNotFound(err) {
			// Resource not found; could have been deleted after reconcile request.
			// Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Paladin resource")
		return ctrl.Result{}, err
	}

	// Initialize status if empty
	if node.Status.Phase == "" {
		node.Status.Phase = corev1alpha1.StatusPhaseFailed
	}

	defer func() {
		// Update the overall phase based on conditions
		if err := r.Status().Update(ctx, &node); err != nil {
			log.Error(err, "Failed to update Paladin status")
		}
	}()

	// Create ConfigMap
	configSum, tlsSecrets, _, err := r.createConfigMap(ctx, &node, name)
	if err != nil {
		log.Error(err, "Failed to create Paladin config map")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionFalse, corev1alpha1.ReasonCMCreationFailed, err.Error())
		return ctrl.Result{}, err
	}
	log.Info("Created Paladin config map", "Name", name)

	// Create Service
	if _, err := r.createService(ctx, &node, name); err != nil {
		log.Error(err, "Failed to create Paladin Service")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSVC, metav1.ConditionFalse, corev1alpha1.ReasonSVCCreationFailed, err.Error())
		return ctrl.Result{}, err
	}
	log.Info("Created Paladin Service", "Name", name)

	// Create Pod Disruption Budget
	if _, err := r.createPDB(ctx, &node, name); err != nil {
		log.Error(err, "Failed to create Paladin pod disruption budget")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionFalse, corev1alpha1.ReasonPDBCreationFailed, err.Error())
		return ctrl.Result{}, err
	}
	log.Info("Created Paladin pod disruption budget", "Name", name)

	// Create StatefulSet
	ss, err := r.createStatefulSet(ctx, &node, name, tlsSecrets, configSum)
	if err != nil {
		log.Error(err, "Failed to create Paladin StatefulSet")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionFalse, corev1alpha1.ReasonSSCreationFailed, err.Error())
		return ctrl.Result{}, err
	}
	log.Info("Created Paladin StatefulSet", "Name", ss.Name, "Namespace", ss.Namespace)

	// Update condition to Succeeded
	node.Status.Phase = corev1alpha1.StatusPhaseCompleted

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PaladinReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Load the controller configuration
	config, err := config.LoadConfig() // Load the config from file
	if err != nil {
		return err
	}
	r.config = config

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Paladin{}).
		// reconcile all paladin nodes, for any change to any domain
		Watches(&corev1alpha1.PaladinDomain{}, reconcileAll(PaladinCRMap, r.Client), reconcileEveryChange()).
		// reconcile all paladin nodes, for any change to any registry
		Watches(&corev1alpha1.PaladinRegistry{}, reconcileAll(PaladinCRMap, r.Client), reconcileEveryChange()).
		Complete(r)
}

func (r *PaladinReconciler) createStatefulSet(ctx context.Context, node *corev1alpha1.Paladin, name string, tlsSecrets []string, configSum string) (*appsv1.StatefulSet, error) {
	statefulSet := r.generateStatefulSetTemplate(node, name, configSum)
	if err := controllerutil.SetControllerReference(node, statefulSet, r.Scheme); err != nil {
		return nil, err
	}

	if node.Spec.Database.Mode == corev1alpha1.DBMode_SidecarPostgres {
		// Earlier processing responsible for ensuring this is non-nil
		r.addPostgresSidecar(statefulSet, *node.Spec.Database.PasswordSecret)
		if err := r.createPostgresPVC(ctx, node, name); err != nil {
			setCondition(&node.Status.Conditions, corev1alpha1.ConditionPVC, metav1.ConditionTrue, corev1alpha1.ReasonPVCCreationFailed, err.Error())
			return nil, err
		}
	}

	// Used by Postgres sidecar, but also custom DB creation - a DB secret needs wiring up to env vars for DSNParams
	if node.Spec.Database.PasswordSecret != nil {
		if err := r.addPaladinDBSecret(ctx, statefulSet, *node.Spec.Database.PasswordSecret); err != nil {
			return nil, err
		}
	}

	r.addKeystoreSecretMounts(statefulSet, node.Spec.SecretBackedSigners)

	r.addTLSSecretMounts(statefulSet, tlsSecrets)

	// Check if the StatefulSet already exists, create if not
	var foundStatefulSet appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, &foundStatefulSet); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, statefulSet)
		if err != nil {
			return statefulSet, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionTrue, corev1alpha1.ReasonSSCreated, fmt.Sprintf("Name: %s", statefulSet.Name))
	} else if err != nil {
		return statefulSet, err
	} else {
		// Only update safe things
		foundStatefulSet.Spec.Template.Spec.Containers = statefulSet.Spec.Template.Spec.Containers
		foundStatefulSet.Spec.Template.Spec.Volumes = statefulSet.Spec.Template.Spec.Volumes
		foundStatefulSet.Spec.Template.Annotations = statefulSet.Spec.Template.Annotations
		foundStatefulSet.Spec.Template.Labels = statefulSet.Spec.Template.Labels
		// TODO: Other things that can be merged?
		if err := r.Update(ctx, &foundStatefulSet); err != nil {
			return statefulSet, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionTrue, corev1alpha1.ReasonSSUpdated, fmt.Sprintf("Name: %s", statefulSet.Name))
		statefulSet = &foundStatefulSet
	}
	return statefulSet, nil
}

func (r *PaladinReconciler) withStandardAnnotations(annotations map[string]string) map[string]string {
	for k, v := range r.config.Paladin.Annotations {
		annotations[k] = v
	}
	return annotations
}

func buildEnv(envMaps ...map[string]string) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0)
	for _, envMap := range envMaps {
		for k, v := range envMap {
			envVars = append(envVars, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
	}
	return envVars
}

func (r *PaladinReconciler) generatePDBTemplate(node *corev1alpha1.Paladin, name string) *policyv1.PodDisruptionBudget {
	// We only have once replica per statefulset, so eviction for upgrade makes sense as the default
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			UnhealthyPodEvictionPolicy: ptrTo(policyv1.AlwaysAllow),
			Selector: &metav1.LabelSelector{
				MatchLabels: r.getLabels(node),
			},
		},
	}
}

func (r *PaladinReconciler) createPDB(ctx context.Context, node *corev1alpha1.Paladin, name string) (*policyv1.PodDisruptionBudget, error) {
	pdb := r.generatePDBTemplate(node, name)
	if err := controllerutil.SetControllerReference(node, pdb, r.Scheme); err != nil {
		return nil, err
	}

	var foundPDB policyv1.PodDisruptionBudget
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: pdb.Namespace}, &foundPDB); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, pdb)
		if err != nil {
			return pdb, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionTrue, corev1alpha1.ReasonPDBCreated, fmt.Sprintf("Name: %s", pdb.Name))
	} else if err != nil {
		return nil, err
	} else {
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionTrue, corev1alpha1.ReasonPDBCreated, fmt.Sprintf("Name: %s", pdb.Name))
	}
	return &foundPDB, nil
}

func (r *PaladinReconciler) generateStatefulSetTemplate(node *corev1alpha1.Paladin, name, configSum string) *appsv1.StatefulSet {
	// Define the StatefulSet to run Paladin using the ConfigMap
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
			Annotations: r.withStandardAnnotations(map[string]string{
				"kubectl.kubernetes.io/default-container": "paladin",
			}),
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: r.getLabels(node),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.getLabels(node),
					Annotations: r.withStandardAnnotations(map[string]string{
						"core.paladin.io/config-sum": fmt.Sprintf("md5-%s", configSum),
					}),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "paladin",
							Image:           r.config.Paladin.Image, // Use the image from the config
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/app/config",
									ReadOnly:  true,
								},
							},
							Args: []string{
								"/app/config/pldconf.paladin.yaml",
								"testbed",
								"--logtostderr=true",
								"--v=4",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "rpc-http",
									ContainerPort: 8548,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "rpc-ws",
									ContainerPort: 8549,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: buildEnv(r.config.Paladin.Envs),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: name,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *PaladinReconciler) addPostgresSidecar(ss *appsv1.StatefulSet, passwordSecret string) {
	ss.Spec.Template.Spec.Containers = append(ss.Spec.Template.Spec.Containers, corev1.Container{
		Name:            "postgres",
		Image:           r.config.Postgres.Image, // Use the image from the config
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "pgdata",
				MountPath: "/pgdata",
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "postgres",
				ContainerPort: 5432,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: append([]corev1.EnvVar{
			{
				Name: "POSTGRES_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: passwordSecret,
						},
						Key: "password",
					},
				},
			},
		}, buildEnv(r.config.Postgres.Envs, map[string]string{
			"PGDATA": "/pgdata",
		})...),
	})
	ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "pgdata",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: fmt.Sprintf("%s-pgdata", ss.Name),
			},
		},
	})
}

func (r *PaladinReconciler) createPostgresPVC(ctx context.Context, node *corev1alpha1.Paladin, name string) error {
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pgdata", name),
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
		},
		Spec: node.Spec.Database.PVCTemplate,
	}
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteOnce,
	}
	if pvc.Spec.Resources.Requests == nil {
		pvc.Spec.Resources.Requests = corev1.ResourceList{}
	}
	if _, resourceSet := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; !resourceSet {
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("1Gi")
	}
	if err := controllerutil.SetControllerReference(node, &pvc, r.Scheme); err != nil {
		return err
	}

	var foundPVC corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &foundPVC); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, &pvc)
		if err != nil {
			return err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPVC, metav1.ConditionTrue, corev1alpha1.ReasonPVCCreated, fmt.Sprintf("Name: %s", pvc.Name))
	} else if err != nil {
		return err
	}
	return nil
}

func (r *PaladinReconciler) addKeystoreSecretMounts(ss *appsv1.StatefulSet, signers []corev1alpha1.SecretBackedSigner) {
	paladinContainer := &ss.Spec.Template.Spec.Containers[0]
	for _, s := range signers {
		paladinContainer.VolumeMounts = append(paladinContainer.VolumeMounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("keystore-%s", s.Name),
			MountPath: fmt.Sprintf("/keystores/%s", s.Name),
		})
		ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: fmt.Sprintf("keystore-%s", s.Name),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: s.Secret,
				},
			},
		})
	}
}

func (r *PaladinReconciler) addTLSSecretMounts(ss *appsv1.StatefulSet, tlsSecrets []string) {
	paladinContainer := &ss.Spec.Template.Spec.Containers[0]
	for tlsIdx, tlsSecretName := range tlsSecrets {
		paladinContainer.VolumeMounts = append(paladinContainer.VolumeMounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("cert%.3d", tlsIdx),
			MountPath: fmt.Sprintf("/cert%.3d", tlsIdx),
		})
		ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: fmt.Sprintf("cert%.3d", tlsIdx),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		})
	}
}

func (r *PaladinReconciler) addPaladinDBSecret(ctx context.Context, ss *appsv1.StatefulSet, secretName string) error {
	_, _, err := r.retrieveUsernamePasswordSecret(ctx, ss.Namespace, secretName)
	if err != nil {
		return fmt.Errorf("failed to extract username/password from DB password secret '%s': %s", secretName, err)
	}

	paladinContainer := &ss.Spec.Template.Spec.Containers[0]
	paladinContainer.VolumeMounts = append(paladinContainer.VolumeMounts, corev1.VolumeMount{
		Name:      "db-creds",
		MountPath: "/db-creds",
	})
	ss.Spec.Template.Spec.Volumes = append(ss.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "db-creds",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	})

	return nil
}

func (r *PaladinReconciler) createConfigMap(ctx context.Context, node *corev1alpha1.Paladin, name string) (string, []string, *corev1.ConfigMap, error) {
	configSum, tlsSecrets, configMap, err := r.generateConfigMap(ctx, node, name)
	if err != nil {
		return "", nil, nil, err
	}
	if err := controllerutil.SetControllerReference(node, configMap, r.Scheme); err != nil {
		return "", nil, nil, err
	}

	var foundConfigMap corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, &foundConfigMap); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, configMap)
		if err != nil {
			return "", nil, nil, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionTrue, corev1alpha1.ReasonCMCreated, fmt.Sprintf("Name: %s", configMap.Name))
	} else if err != nil {
		return "", nil, nil, err
	} else {
		foundConfigMap.Data = configMap.Data
		if err := r.Update(ctx, &foundConfigMap); err != nil {
			return "", nil, nil, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionTrue, corev1alpha1.ReasonCMUpdated, fmt.Sprintf("Name: %s", configMap.Name))
		configMap = &foundConfigMap
	}
	return configSum, tlsSecrets, configMap, nil
}

// generatePaladinConfigMapTemplate generates a ConfigMap for the Paladin configuration
func (r *PaladinReconciler) generateConfigMap(ctx context.Context, node *corev1alpha1.Paladin, name string) (string, []string, *corev1.ConfigMap, error) {
	pldConfigYAML, tlsSecrets, err := r.generatePaladinConfig(ctx, node, name)
	if err != nil {
		return "", nil, nil, err
	}
	configSum := md5.New()
	configSum.Write([]byte(pldConfigYAML))
	configSumHex := hex.EncodeToString(configSum.Sum(nil))
	return configSumHex,
		tlsSecrets,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: node.Namespace,
				Labels:    r.getLabels(node),
			},
			Data: map[string]string{
				"pldconf.paladin.yaml": pldConfigYAML,
			},
		},
		nil
}

// generatePaladinConfig converts the Paladin CR spec to a Paladin YAML configuration
func (r *PaladinReconciler) generatePaladinConfig(ctx context.Context, node *corev1alpha1.Paladin, name string) (string, []string, error) {
	var pldConf pldconf.PaladinConfig
	if node.Spec.Config != nil {
		err := yaml.Unmarshal([]byte(*node.Spec.Config), &pldConf)
		if err != nil {
			return "", nil, fmt.Errorf("paladinConfigYAML is invalid: %s", err)
		}
	}

	// Node name can be overridden, but defaults to the CR name
	if pldConf.NodeName == "" {
		pldConf.NodeName = node.Name
	}

	// Merge in the ports for our networking config (which we need to match our service defs)
	pldConf.RPCServer.HTTP.Port = ptrTo(8548)
	pldConf.RPCServer.HTTP.Address = ptrTo("0.0.0.0") // use k8s for network control outside the pod
	pldConf.RPCServer.WS.Port = ptrTo(8549)
	pldConf.RPCServer.WS.Address = ptrTo("0.0.0.0") // use k8s for network control outside the pod

	// DB needs merging from user config and our config
	if err := r.generatePaladinDBConfig(ctx, node, &pldConf, name); err != nil {
		return "", nil, err
	}

	// Merge k8s definitions of signers with the supplied config
	if err := r.generatePaladinSigners(ctx, node, &pldConf); err != nil {
		return "", nil, err
	}

	// Merge k8s CR label references to domains, with the supplied static config
	if err := r.generatePaladinDomains(ctx, node, &pldConf); err != nil {
		return "", nil, err
	}

	// Merge k8s CR label references to registries, with the supplied static config
	if err := r.generatePaladinRegistries(ctx, node, &pldConf); err != nil {
		return "", nil, err
	}

	tlsSecrets, err := r.generatePaladinTransports(ctx, node, &pldConf)
	if err != nil {
		return "", nil, err
	}

	// Bind to the a local besu node if we've been configured with one
	if node.Spec.BesuNode != "" {
		pldConf.Blockchain.HTTP.URL = fmt.Sprintf("http://%s:8545", generateBesuServiceHostname(node.Spec.BesuNode, node.Namespace))
		pldConf.Blockchain.WS.URL = fmt.Sprintf("ws://%s:8546", generateBesuServiceHostname(node.Spec.BesuNode, node.Namespace))
	}
	b, err := yaml.Marshal(&pldConf)
	return string(b), tlsSecrets, err
}

func (r *PaladinReconciler) generatePaladinDBConfig(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig, name string) error {
	dbSpec := &node.Spec.Database

	// If we're responsible for the runtime, we need to fill in the URI
	switch dbSpec.Mode {
	case corev1alpha1.DBMode_EmbeddedSQLite:
		pldConf.DB.Type = "sqlite"
		// We mix in our URI with whatever additional config has already been provided via YAML
		pldConf.DB.SQLite.SQLDBConfig.DSN = "file:/db/db"
	case corev1alpha1.DBMode_SidecarPostgres:
		pldConf.DB.Type = "postgres"
		// Note password wrangling happens later
		if dbSpec.PasswordSecret == nil {
			defaultPasswordSecret := fmt.Sprintf("%s-db", name)
			dbSpec.PasswordSecret = &defaultPasswordSecret
		}
		if err := r.generateDBPasswordSecretIfNotExist(ctx, node, *dbSpec.PasswordSecret); err != nil {
			return err
		}
		pldConf.DB.Postgres.SQLDBConfig.DSN = "postgres://{{.username}}:{{.password}}@localhost:5432/postgres?sslmode=disable"
	}

	// Get a handle to the DB config where we'll put password/migration info
	var sqlConfig *pldconf.SQLDBConfig
	switch pldConf.DB.Type {
	case "sqlite":
		sqlConfig = &pldConf.DB.SQLite.SQLDBConfig
	case "postgres":
		sqlConfig = &pldConf.DB.Postgres.SQLDBConfig
	default:
		return fmt.Errorf("unsupported database type in configuration '%s'", pldConf.DB.Type)
	}

	// If we're responsible for loading the password for the Postgres URL, then load it from the secret
	if dbSpec.PasswordSecret != nil {
		if pldConf.DB.Postgres.SQLDBConfig.DSNParams == nil {
			pldConf.DB.Postgres.SQLDBConfig.DSNParams = map[string]pldconf.DSNParamLocation{}
		}
		pldConf.DB.Postgres.SQLDBConfig.DSNParams["username"] = pldconf.DSNParamLocation{File: "/db-creds/username"}
		pldConf.DB.Postgres.SQLDBConfig.DSNParams["password"] = pldconf.DSNParamLocation{File: "/db-creds/password"}
	}

	// If we're responsible for migration settings, then do it
	switch dbSpec.MigrationMode {
	case corev1alpha1.DBMigrationMode_Auto:
		truthy := true
		sqlConfig.AutoMigrate = &truthy
		sqlConfig.MigrationsDir = fmt.Sprintf("/app/db/migrations/%s", pldConf.DB.Type)
	}

	return nil
}

func (r *PaladinReconciler) generateDBPasswordSecretIfNotExist(ctx context.Context, node *corev1alpha1.Paladin, name string) error {
	secret := r.generateSecretTemplate(node, name)
	if err := controllerutil.SetControllerReference(node, secret, r.Scheme); err != nil {
		return err
	}

	var foundSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &foundSecret); err != nil && errors.IsNotFound(err) {
		secret.StringData = map[string]string{
			"username": "postgres",
			"password": randURLSafeBase64(32),
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionTrue, corev1alpha1.ReasonPDBCreated, fmt.Sprintf("Name: %s", name))
	} else if err != nil {
		return err
	}
	return nil
}

func (r *PaladinReconciler) generatePaladinSigners(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig) error {

	for i, s := range node.Spec.SecretBackedSigners {

		// Temporary restriction in Paladin
		if i > 0 {
			return fmt.Errorf("only one signer currently supported")
		}

		wallet := &pldconf.WalletConfig{
			Name:        s.Name,
			SignerType:  pldconf.WalletSignerTypeEmbedded,
			KeySelector: s.KeySelector,
			Signer:      &pldconf.SignerConfig{},
		}

		// Upsert a secret if we've been asked to. We use a mnemonic in this case (rather than directly generating a 32byte seed)
		if s.Type == corev1alpha1.SignerType_AutoHDWallet {
			wallet.Signer.KeyDerivation.Type = pldconf.KeyDerivationTypeBIP32
			wallet.Signer.KeyDerivation.SeedKeyPath = pldconf.SigningKeyConfigEntry{Name: "seed"}
			if err := r.generateBIP39SeedSecretIfNotExist(ctx, node, s.Secret); err != nil {
				return err
			}
		}

		// Note we update the fields we are responsible for, rather than creating the whole object
		// So detailed configuration can be provided in the config YAML
		wallet.Signer.KeyStore.Type = pldconf.KeyStoreTypeStatic
		wallet.Signer.KeyStore.Static.File = fmt.Sprintf("/keystores/%s/keys.yaml", s.Name)

		// Add to the end of the list in the order defined
		pldConf.Wallets = append(pldConf.Wallets, wallet)

	}

	return nil
}

func (r *PaladinReconciler) generatePaladinDomains(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig) error {

	// Use all the label selectors we have to get a deterministically sorted list of CRs
	allResults := corev1alpha1.PaladinDomainList{}
	for i, s := range node.Spec.Domains {
		var results corev1alpha1.PaladinDomainList
		selector, err := metav1.LabelSelectorAsSelector(&s.LabelSelector)
		if err == nil {
			err = r.List(ctx, &results, client.MatchingLabelsSelector{Selector: selector})
		}
		if err != nil {
			return fmt.Errorf("error using label selector at position %d of domains: %s", i, err)
		}
		allResults.Items = append(allResults.Items, results.Items...)
	}
	sortedResults := deDupAndSortInLocalNS(PaladinDomainCRMap, &allResults)

	// Now go through the list sorting out the config
	if pldConf.Domains == nil {
		pldConf.Domains = make(map[string]*pldconf.DomainConfig)
	}
	for _, domain := range sortedResults {
		if domain.Status.Status != corev1alpha1.DomainStatusAvailable {
			log.FromContext(ctx).Info(fmt.Sprintf("configJSON for domain '%s' not ready yet: %s", domain.Name, domain.Status.Status))
			continue // skip it - but continue trying others
		}

		var domainConf map[string]any
		if err := json.Unmarshal([]byte(domain.Spec.ConfigJSON), &domainConf); err != nil {
			log.FromContext(ctx).Error(err, fmt.Sprintf("configJSON for domain '%s' cannot be parsed (skipping)", domain.Name))
			continue // skip it - but continue trying others
		}

		// A domain is either wholely defined in the static config of the node,
		// or wholely defined by reference to the CR attached.
		// We do not attempt to merge
		pldConf.Domains[domain.Name] = &pldconf.DomainConfig{
			Plugin:          r.mapPluginConfig(domain.Spec.Plugin),
			Config:          domainConf,
			RegistryAddress: domain.Status.RegistryAddress,
			AllowSigning:    domain.Spec.AllowSigning,
		}
	}

	return nil
}

func (r *PaladinReconciler) mapPluginConfig(in corev1alpha1.PluginConfig) pldconf.PluginConfig {
	return pldconf.PluginConfig{
		Type:    in.Type,
		Library: in.Library,
		Class:   in.Class,
	}
}

func (r *PaladinReconciler) generatePaladinRegistries(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig) error {

	// Use all the label selectors we have to get a deterministically sorted list of CRs
	allResults := corev1alpha1.PaladinRegistryList{}
	for i, s := range node.Spec.Registries {
		var results corev1alpha1.PaladinRegistryList
		selector, err := metav1.LabelSelectorAsSelector(&s.LabelSelector)
		if err == nil {
			err = r.List(ctx, &results, client.MatchingLabelsSelector{Selector: selector})
		}
		if err != nil {
			return fmt.Errorf("error using label selector at position %d of domains: %s", i, err)
		}
		allResults.Items = append(allResults.Items, results.Items...)
	}
	sortedResults := deDupAndSortInLocalNS(PaladinRegistryCRMap, &allResults)

	// Now go through the list sorting out the config
	if pldConf.Registries == nil {
		pldConf.Registries = make(map[string]*pldconf.RegistryConfig)
	}
	for _, reg := range sortedResults {
		if reg.Status.Status != corev1alpha1.RegistryStatusAvailable {
			log.FromContext(ctx).Info(fmt.Sprintf("configJSON for registry '%s' not ready yet: %s", reg.Name, reg.Status.Status))
			continue // skip it - but continue trying others
		}

		var registryConf map[string]any
		if err := json.Unmarshal([]byte(reg.Spec.ConfigJSON), &registryConf); err != nil {
			log.FromContext(ctx).Error(err, fmt.Sprintf("configJSON for registry '%s' cannot be parsed (skipping)", reg.Name))
			continue // skip it - but continue trying others
		}
		if reg.Spec.Type == corev1alpha1.RegistryTypeEVM {
			registryConf["contractAddress"] = reg.Status.ContractAddress
		}

		// A domain is either wholely defined in the static config of the node,
		// or wholely defined by reference to the CR attached.
		// We do not attempt to merge
		pldConf.Registries[reg.Name] = &pldconf.RegistryConfig{
			Plugin: r.mapPluginConfig(reg.Spec.Plugin),
			Transports: pldconf.RegistryTransportsConfig{
				Enabled:           reg.Spec.Transports.Enabled,
				RequiredPrefix:    reg.Spec.Transports.RequiredPrefix,
				HierarchySplitter: reg.Spec.Transports.HierarchySplitter,
				PropertyRegexp:    reg.Spec.Transports.PropertyRegexp,
				TransportMap:      reg.Spec.Transports.TransportMap,
			},
			Config: registryConf,
		}
	}

	return nil
}

func (r *PaladinReconciler) generatePaladinTransports(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig) ([]string, error) {
	availableTLSSecrets := []string{}
	for _, transport := range node.Spec.Transports {

		var transportConf map[string]any
		if err := json.Unmarshal([]byte(transport.ConfigJSON), &transportConf); err != nil {
			log.FromContext(ctx).Error(err, fmt.Sprintf("configJSON for transport '%s' cannot be parsed (skipping)", transport.Name))
			continue // skip it - but continue trying others
		}

		if transport.TLS.SecretName == "" {
			// No TLS config - we can skip this one
			// TODO: Is this acceptable?
			continue
		}

		// See if the secret is available
		var secret corev1.Secret
		err := r.Get(ctx, types.NamespacedName{Name: transport.TLS.SecretName, Namespace: node.Namespace}, &secret)
		if err == nil {
			// The secret is there - all good we can continue
		} else if errors.IsNotFound(err) {
			if transport.TLS.CertName != "" {
				// We need to ensure we have sent the issuance request
				if err := r.createCertificate(ctx, node, pldConf, &transport.TLS); err != nil {
					return nil, err
				}
			}
			// We return an error here to force a re-reconcile while we wait for the certificate
			return nil, fmt.Errorf("waiting for secret '%s' to be available for transport '%s'", transport.TLS.SecretName, transport.Name)
		} else {
			return nil, fmt.Errorf("failed to lookup secret for transport %s: %s", transport.Name, err)
		}

		tlsIdx := len(availableTLSSecrets)
		availableTLSSecrets = append(availableTLSSecrets, secret.Name)
		transportConf["tls"] = &pldconf.TLSConfig{
			Enabled:    true,
			ClientAuth: true,
			CertFile:   fmt.Sprintf("/cert%.3d/tls.crt", tlsIdx),
			KeyFile:    fmt.Sprintf("/cert%.3d/tls.key", tlsIdx),
		}
		if transportConf["externalHostname"] == nil {
			transportConf["externalHostname"] = generatePaladinServiceHostname(node.Name, node.Namespace)
		}

		// It's available, add it to our config
		if pldConf.Transports == nil {
			pldConf.Transports = make(map[string]*pldconf.TransportConfig)
		}
		pldConf.Transports[transport.Name] = &pldconf.TransportConfig{
			Plugin: r.mapPluginConfig(transport.Plugin),
			Config: transportConf,
		}

	}

	return availableTLSSecrets, nil
}

func (r *PaladinReconciler) createCertificate(ctx context.Context, node *corev1alpha1.Paladin, pldConf *pldconf.PaladinConfig, tlsConf *corev1alpha1.TLSConfig) error {

	var certUnstructured unstructured.Unstructured
	certUnstructured.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Certificate",
	})
	err := r.Client.Get(ctx, types.NamespacedName{Name: tlsConf.CertName, Namespace: node.Namespace}, &certUnstructured)
	if err == nil {
		// already exists - we do not support modifying it currently
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	certUnstructured.SetName(tlsConf.CertName)
	certUnstructured.SetNamespace(node.Namespace)
	certUnstructured.SetLabels(r.getLabels(node))

	// Build the DNS name for the node
	dnsNames := append([]string{generatePaladinServiceHostname(node.Name, node.Namespace)}, tlsConf.AdditionalDNSNames...)

	// We use a template to generate it
	const defaultTemplate = `commonName: {{ .nodeName }}
secretName: {{ .secretName }}
dnsNames: {{ .dnsNames | toJson }}
issuerRef:
  kind: Issuer
  name: {{ .issuer }}
`
	specTemplate := defaultTemplate
	if tlsConf.CertSpecTemplate != "" {
		specTemplate = tlsConf.CertSpecTemplate
	}
	template, err := template.New("").Option("missingkey=error").Funcs(sprig.FuncMap()).Parse(specTemplate)
	if err != nil {
		return fmt.Errorf("invalid certSpecTemplate: %s", err)
	}
	yamlBuff := new(bytes.Buffer)
	err = template.Execute(yamlBuff, map[string]any{
		"nodeName":   pldConf.NodeName,
		"secretName": tlsConf.SecretName,
		"dnsNames":   dnsNames,
		"issuer":     tlsConf.Issuer,
	})
	var yamlMap map[string]any
	if err == nil {
		err = yaml.Unmarshal(yamlBuff.Bytes(), &yamlMap)
	}
	if err != nil {
		return fmt.Errorf("failed to execute certSpecTemplate: %s", err)
	}
	certUnstructured.Object["spec"] = yamlMap

	if err := controllerutil.SetControllerReference(node, &certUnstructured, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, &certUnstructured)

}

func (r *PaladinReconciler) generateBIP39SeedSecretIfNotExist(ctx context.Context, node *corev1alpha1.Paladin, name string) error {
	secret := r.generateSecretTemplate(node, name)
	if err := controllerutil.SetControllerReference(node, secret, r.Scheme); err != nil {
		return err
	}

	var foundSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &foundSecret); err != nil && errors.IsNotFound(err) {
		var keyEntryJSON []byte
		var mnemonic string
		entropy, err := bip39.NewEntropy(256)
		if err == nil {
			mnemonic, err = bip39.NewMnemonic(entropy)
		}
		if err == nil {
			keyEntryJSON, err = json.MarshalIndent(map[string]pldconf.StaticKeyEntryConfig{
				"seed": {
					Encoding: "none",
					Inline:   mnemonic,
				},
			}, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("failed to generate mnemonic: %s", err)
		}
		secret.StringData = map[string]string{
			"keys.yaml": string(keyEntryJSON),
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func (r *PaladinReconciler) retrieveUsernamePasswordSecret(ctx context.Context, namespace, name string) (string, string, error) {
	var foundSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &foundSecret); err != nil {
		return "", "", err
	}
	return string(foundSecret.Data["username"]), string(foundSecret.Data["password"]), nil
}

func (r *PaladinReconciler) createService(ctx context.Context, node *corev1alpha1.Paladin, name string) (*corev1.Service, error) {
	svc := r.generateServiceTemplate(node, name)
	if err := controllerutil.SetControllerReference(node, svc, r.Scheme); err != nil {
		return svc, err
	}
	for _, transport := range node.Spec.Transports {
		svc.Spec.Ports = append(svc.Spec.Ports, transport.Ports...)
	}

	var foundSvc corev1.Service
	if err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &foundSvc); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, svc)
		if err != nil {
			return svc, fmt.Errorf("failed to create service: %s", err)
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSVC, metav1.ConditionTrue, corev1alpha1.ReasonSVCCreated, fmt.Sprintf("Name: %s", name))
	} else if err != nil {
		return svc, err
	}
	foundSvc.Spec = svc.Spec
	return &foundSvc, r.Update(ctx, &foundSvc)
}

// generateServiceTemplate generates a ConfigMap for the Paladin configuration
func (r *PaladinReconciler) generateServiceTemplate(node *corev1alpha1.Paladin, name string) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
		},
		Spec: node.Spec.Service,
	}
	// We own the selector regardless of config in the CR
	svc.Spec.Selector = r.getLabels(node)
	// Default to a cluster IP
	if svc.Spec.Type == "" {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}
	// Merge our required ports with the overrides the user has provided
	mergeServicePorts(&svc.Spec, []corev1.ServicePort{
		{
			Name:       "rpc-http",
			Port:       8548,
			TargetPort: intstr.FromInt(8548),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "rpc-ws",
			Port:       8549,
			TargetPort: intstr.FromInt(8549),
			Protocol:   corev1.ProtocolTCP,
		},
	})
	return svc
}

func randURLSafeBase64(count int) string {
	bytes := make([]byte, count)
	_, _ = rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}

// generateSecretTemplate generates a Secret for the Paladin configuration
func (r *PaladinReconciler) generateSecretTemplate(node *corev1alpha1.Paladin, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
		},
	}
}

func (r *PaladinReconciler) getLabels(node *corev1alpha1.Paladin, extraLabels ...map[string]string) map[string]string {
	l := make(map[string]string, len(r.config.Paladin.Labels))
	for k, v := range r.config.Paladin.Labels {
		l[k] = v
	}
	for _, e := range extraLabels {
		for k, v := range e {
			l[k] = v
		}
	}
	l["app.kubernetes.io/name"] = generatePaladinName(node.Name)
	return l
}

// generatePaladinName generates a name for the Paladin resources based on the Paladin name.
// this is for generating unique names for the resources
func generatePaladinName(n string) string {
	return fmt.Sprintf("paladin-%s", n)
}

func generatePaladinServiceHostname(nodeName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", generatePaladinName(nodeName), namespace)
}

func getPaladinURLEndpoint(ctx context.Context, c client.Client, name, namespace string) (string, error) {
	if os.Getenv("USE_NODEPORTS_IN_DEBUGGER") == "true" {
		// Running outside of kubernetes, so we require a nodeport to be defined and exposed from Kind
		var svc corev1.Service
		err := c.Get(ctx, types.NamespacedName{Name: generatePaladinName(name), Namespace: namespace}, &svc)
		if err != nil {
			return "", fmt.Errorf("failed to find service for node: %s", err)
		}
		for _, p := range svc.Spec.Ports {
			if p.Name == "rpc-http" {
				if p.NodePort == 0 {
					return "", fmt.Errorf("cannot use nodePort for %s rpc-http port as not cset", generatePaladinName(name))
				}
				return fmt.Sprintf("http://localhost:%d", p.NodePort), nil
			}
		}
		return "", fmt.Errorf("did not find rpc-http port")
	}
	// standard path
	return fmt.Sprintf("http://%s:8548", generatePaladinServiceHostname(name, namespace)), nil
}
