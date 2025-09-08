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
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/operator/pkg/config"
	"github.com/hyperledger/firefly-signer/pkg/secp256k1"
	"github.com/pelletier/go-toml/v2"
)

// BesuReconciler reconciles a Besu object
type BesuReconciler struct {
	client.Client
	config *config.Config
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var BesuCRMap = CRMap[corev1alpha1.Besu, *corev1alpha1.Besu, *corev1alpha1.BesuList]{
	NewList:  func() *corev1alpha1.BesuList { return new(corev1alpha1.BesuList) },
	ItemsFor: func(list *corev1alpha1.BesuList) []corev1alpha1.Besu { return list.Items },
	AsObject: func(item *corev1alpha1.Besu) *corev1alpha1.Besu { return item },
}

// Reconcile implements the logic when a Besu resource is created, updated, or deleted
func (r *BesuReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Generate a name for the Besu resources
	name := generateBesuName(req.Name)

	// Fetch the Besu instance
	var node corev1alpha1.Besu
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if errors.IsNotFound(err) {
			// Resource not found; could have been deleted after reconcile request.
			// Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Besu resource")
		return ctrl.Result{}, err
	}

	// Initialize status if empty
	if node.Status.Phase == "" {
		node.Status.Phase = corev1alpha1.StatusPhaseFailed
	}

	defer func() {
		// Update the overall phase based on conditions
		if err := r.Status().Update(ctx, &node); err != nil {
			if errors.IsConflict(err) {
				log.Info("Conflict updating Besu status")
			} else {
				log.Error(err, "Failed to update Besu status")
			}
		}
	}()

	// Create Identity Secret
	if _, err := r.createIdentitySecret(ctx, &node); err != nil {
		log.Error(err, "Failed to create Besu identity secret")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSecret, metav1.ConditionFalse, corev1alpha1.ReasonSecretCreationFailed, err.Error())
		return ctrl.Result{}, err
	}
	log.Info("Created Besu identity secret", "Name", name)

	// Load Genesis
	genesis, err := r.loadGenesis(ctx, &node)
	if err != nil {
		log.Error(err, "Failed to retrieve BesuGenesis")
		node.Status.Phase = corev1alpha1.StatusPhasePending
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionGenesisAvailable, metav1.ConditionFalse, corev1alpha1.ReasonGenesisNotFound, err.Error())
		return ctrl.Result{}, err
	}
	if genesis == nil {
		log.Info("Waiting for genesis to become available")
		node.Status.Phase = corev1alpha1.StatusPhasePending
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionGenesisAvailable, metav1.ConditionFalse, corev1alpha1.ReasonGenesisNotFound, "Genesis resource not found")
		// Requeue after a delay
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}
	// Genesis is available
	setCondition(&node.Status.Conditions, corev1alpha1.ConditionGenesisAvailable, metav1.ConditionTrue, corev1alpha1.ReasonSuccess, "Genesis resource is available")

	// Create ConfigMap
	configSum, _, err := r.createConfigMap(ctx, &node)
	if err != nil {
		log.Error(err, "Failed to create Besu config map")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionFalse, corev1alpha1.ReasonCMCreationFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Create Service
	if _, err := r.createService(ctx, &node, name); err != nil {
		log.Error(err, "Failed to create Besu Service")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSVC, metav1.ConditionFalse, corev1alpha1.ReasonSVCCreationFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Create Pod Disruption Budget
	if _, err := r.createPDB(ctx, &node, name); err != nil {
		log.Error(err, "Failed to create Besu pod disruption budget")
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionFalse, corev1alpha1.ReasonPDBCreationFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Create StatefulSet
	if _, err := r.createStatefulSet(ctx, &node, name, configSum); err != nil {
		log.Error(err, "Failed to create Besu StatefulSet")
		setCondition(&genesis.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionFalse, corev1alpha1.ReasonSSCreationFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Update condition to Succeeded
	node.Status.Phase = corev1alpha1.StatusPhaseReady

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BesuReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Load the controller configuration
	config, err := config.LoadConfig() // Load the config from file
	if err != nil {
		return err
	}
	r.config = config

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Besu{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}

func (r *BesuReconciler) createConfigMap(ctx context.Context, node *corev1alpha1.Besu) (string, *corev1.ConfigMap, error) {
	configSum, configMap, err := r.generateConfigMap(ctx, node)
	if err != nil {
		return "", nil, err
	}

	if err := controllerutil.SetControllerReference(node, configMap, r.Scheme); err != nil {
		return "", nil, err
	}

	var foundConfigMap corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, &foundConfigMap); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, configMap)
		if err != nil {
			return "", nil, err
		}
		log.FromContext(ctx).Info("Created Besu config map", "Name", configMap.Name)
	} else if err != nil {
		return "", nil, err
	} else {
		foundConfigMap.Data = configMap.Data
		return configSum, &foundConfigMap, r.Update(ctx, &foundConfigMap)
	}
	return configSum, configMap, nil
}

func (r *BesuReconciler) generateConfigMap(ctx context.Context, node *corev1alpha1.Besu) (string, *corev1.ConfigMap, error) {
	besuConfigTOML, err := r.generateBesuConfigTOML(node)
	if err != nil {
		return "", nil, err
	}

	staticNodesJSON, err := r.generateStaticNodesJSON(ctx, node)
	if err != nil {
		return "", nil, err
	}

	configSum := md5.New()
	configSum.Write([]byte(besuConfigTOML))
	configSum.Write([]byte(staticNodesJSON))
	configSumHex := hex.EncodeToString(configSum.Sum(nil))
	return configSumHex,
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateBesuName(node.Name),
				Namespace: node.Namespace,
				Labels:    r.getLabels(node),
			},
			Data: map[string]string{
				"pldconf.besu.toml": besuConfigTOML,
				"static-nodes.json": staticNodesJSON,
			},
		},
		nil
}

// generateBesuConfigTOML converts the Besu CR spec to a Besu TOML configuration
func (r *BesuReconciler) generateBesuConfigTOML(node *corev1alpha1.Besu) (string, error) {
	tomlConfig := map[string]any{}

	setIfUnset := func(k string, v any) {
		if _, isSet := tomlConfig[k]; !isSet {
			tomlConfig[k] = v
		}
	}

	if node.Spec.Config != nil {
		if err := toml.Unmarshal([]byte(*node.Spec.Config), &tomlConfig); err != nil {
			return "", fmt.Errorf("failed to parse supplied Besu config as TOML: %s", err)
		}
	}
	const localhost = "0.0.0.0"

	// Setup from our mounts
	tomlConfig["node-private-key-file"] = "/nodeid/key"
	tomlConfig["data-path"] = "/data"
	tomlConfig["genesis-file"] = "/genesis/genesis.json"

	// Set up the networking, as that's always in our control (we wire it up to the service)
	comprehensiveRPCSet := []string{"ETH", "NET", "QBFT", "WEB3", "ADMIN", "DEBUG", "TXPOOL"}
	tomlConfig["rpc-http-enabled"] = true
	tomlConfig["rpc-http-host"] = localhost
	tomlConfig["rpc-http-port"] = "8545"
	setIfUnset("rpc-http-api", comprehensiveRPCSet)
	tomlConfig["rpc-ws-enabled"] = true
	tomlConfig["rpc-ws-host"] = localhost
	tomlConfig["rpc-ws-port"] = "8546"
	setIfUnset("rpc-ws-api", comprehensiveRPCSet)
	tomlConfig["graphql-http-enabled"] = true
	tomlConfig["graphql-http-host"] = localhost
	tomlConfig["graphql-http-port"] = "8547"
	setIfUnset("min-gas-price", 0)
	tomlConfig["p2p-host"] = localhost
	tomlConfig["p2p-port"] = "30303"
	setIfUnset("host-allowlist", []string{"*"})

	setIfUnset("logging", "DEBUG")
	setIfUnset("revert-reason-enabled", true)
	setIfUnset("tx-pool", "SEQUENCED")
	setIfUnset("tx-pool-limit-by-account-percentage", 1.0)

	// To give a stable network through node restarts we use hostnames in static-nodes.json
	// https://besu.hyperledger.org/24.1.0/public-networks/concepts/node-keys#enode-url
	tomlConfig["discovery-enabled"] = false
	tomlConfig["static-nodes-file"] = "/config/static-nodes.json"
	tomlConfig["Xdns-enabled"] = true
	tomlConfig["Xdns-update-enabled"] = true

	sb := new(strings.Builder)
	_ = toml.NewEncoder(sb).Encode(tomlConfig)
	return sb.String(), nil
}

func (r *BesuReconciler) generateStaticNodesJSON(ctx context.Context, node *corev1alpha1.Besu) (string, error) {

	// Find all node identities with the same genesis
	// We use a label to mark all the secrets that should be node ID
	sameGenesis, _ := labels.NewRequirement("besu-genesis", selection.Equals, []string{node.Spec.Genesis})
	withNodeName, _ := labels.NewRequirement("besu-node-id", selection.Exists, []string{})
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets, client.InNamespace(node.Namespace), client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*sameGenesis, *withNodeName),
	}); err != nil {
		return "", err
	}

	enodeURLs := []string{}
	for _, nodeIdSecret := range secrets.Items {
		// note it's perfectly fine to include our own node in the static-nodes.json
		nodeName := nodeIdSecret.Labels["besu-node-id"]
		enodeURLs = append(enodeURLs, fmt.Sprintf("enode://%s@%s:30303",
			nodeIdSecret.Data["id"], generateBesuServiceHostname(nodeName, node.Namespace)))
	}

	// Need a consistent hash
	sort.Strings(enodeURLs)

	b, _ := json.MarshalIndent(enodeURLs, "", "  ")
	return string(b), nil
}

func generateBesuServiceHostname(nodeName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", generateBesuName(nodeName), namespace)
}

// generateBesuName generates a name for the Besu resources based on the Besu name.
// this is for generating unique names for the resources
func generateBesuName(n string) string {
	return fmt.Sprintf("besu-%s", n)
}

func generateBesuPVCName(n string) string {
	return fmt.Sprintf("%s-data", generateBesuName(n))
}

func generateBesuIDSecretName(n string) string {
	return fmt.Sprintf("besu-%s-id", n)
}

func (r *BesuReconciler) getLabels(node *corev1alpha1.Besu, extraLabels ...map[string]string) map[string]string {
	l := make(map[string]string, len(r.config.Besu.Labels))

	for k, v := range r.config.Besu.Labels {
		l[k] = v
	}

	for _, e := range extraLabels {
		for k, v := range e {
			l[k] = v
		}
	}
	l["app.kubernetes.io/name"] = generateBesuName(node.Name)
	l["app.kubernetes.io/instance"] = node.Name
	l["app.kubernetes.io/part-of"] = "paladin"

	return l
}

func (r *BesuReconciler) createIdentitySecret(ctx context.Context, node *corev1alpha1.Besu) (*corev1.Secret, error) {
	name := generateBesuIDSecretName(node.Name)

	var idSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: node.Namespace}, &idSecret); err != nil && errors.IsNotFound(err) {
		idSecret = *r.generateSecretTemplate(node, name)
		idSecret.Labels["besu-node-id"] = node.Name
		idSecret.Labels["besu-genesis"] = node.Spec.Genesis
		randBytes32 := make([]byte, 32)
		_, _ = rand.Read(randBytes32)
		nodeKey := secp256k1.KeyPairFromBytes(randBytes32)
		idSecret.StringData = map[string]string{
			"key":     hex.EncodeToString(randBytes32),
			"id":      hex.EncodeToString(nodeKey.PublicKeyBytes()),
			"address": nodeKey.Address.String(),
		}
		if err := controllerutil.SetControllerReference(node, &idSecret, r.Scheme); err != nil {
			return nil, err
		}

		err = r.Create(ctx, &idSecret)
		if err != nil {
			return nil, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionTrue, corev1alpha1.ReasonSecretCreated, fmt.Sprintf("Name: %s", name))
	} else if err != nil {
		return nil, err
	}

	return &idSecret, nil
}

func (r *BesuReconciler) loadGenesis(ctx context.Context, node *corev1alpha1.Besu) (*corev1alpha1.BesuGenesis, error) {

	// We load the genesis CR definition
	var genesis corev1alpha1.BesuGenesis
	err := r.Get(ctx, types.NamespacedName{Name: node.Spec.Genesis, Namespace: node.Namespace}, &genesis)
	if err == nil {
		// ... and we load the config map which will be generated by the genesis controller once it has read
		// the IDs of any nodes declared in the InitialValidators set (including ours which we just wrote).
		// We do this as we don't want to create the pod until this exits, although we don't use the result directly.
		var cm corev1.ConfigMap
		err = r.Get(ctx, types.NamespacedName{Name: generateBesuGenesisName(node.Spec.Genesis), Namespace: node.Namespace}, &cm)
	}
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &genesis, nil
}

func (r *BesuReconciler) generateSecretTemplate(node *corev1alpha1.Besu, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
		},
	}
}

func (r *BesuReconciler) createStatefulSet(ctx context.Context, node *corev1alpha1.Besu, name, configSum string) (*appsv1.StatefulSet, error) {
	statefulSet := r.generateStatefulSetTemplate(node, name, configSum)

	if err := r.createDataPVC(ctx, node); err != nil {
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPVC, metav1.ConditionTrue, corev1alpha1.ReasonPVCCreationFailed, err.Error())
		return nil, err
	}
	if err := controllerutil.SetControllerReference(node, statefulSet, r.Scheme); err != nil {
		return nil, err
	}

	// Check if the StatefulSet already exists, create if not
	var foundStatefulSet appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, &foundStatefulSet); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, statefulSet)
		if err != nil {
			return statefulSet, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionTrue, corev1alpha1.ReasonSSCreated, fmt.Sprintf("Name: %s", statefulSet.Name))
		log.FromContext(ctx).Info("Created Besu StatefulSet", "Name", statefulSet.Name)
	} else if err != nil {
		return statefulSet, err
	} else {
		rv := foundStatefulSet.ResourceVersion
		// Only update safe things
		foundStatefulSet.Spec.Template.Spec.Containers = statefulSet.Spec.Template.Spec.Containers
		foundStatefulSet.Spec.Template.Spec.Volumes = statefulSet.Spec.Template.Spec.Volumes
		foundStatefulSet.Spec.Template.Annotations = statefulSet.Spec.Template.Annotations
		foundStatefulSet.Spec.Template.Labels = statefulSet.Spec.Template.Labels
		// TODO: Other things that can be merged?
		if err := r.Update(ctx, &foundStatefulSet); err != nil {
			return statefulSet, err
		}
		if rv != foundStatefulSet.ResourceVersion {
			setCondition(&node.Status.Conditions, corev1alpha1.ConditionSS, metav1.ConditionTrue, corev1alpha1.ReasonSSUpdated, fmt.Sprintf("Name: %s", statefulSet.Name))
			log.FromContext(ctx).Info("Updated Besu StatefulSet", "Name", statefulSet.Name)
		}
	}
	return statefulSet, nil
}

func (r *BesuReconciler) createDataPVC(ctx context.Context, node *corev1alpha1.Besu) error {
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateBesuPVCName(node.Name),
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
		},
		Spec: node.Spec.PVCTemplate,
	}

	var foundPVC corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, &foundPVC); err != nil && errors.IsNotFound(err) {
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

		if err = r.Create(ctx, &pvc); err != nil {
			return err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPVC, metav1.ConditionTrue, corev1alpha1.ReasonPVCCreated, fmt.Sprintf("Name: %s", pvc.Name))
	} else if err != nil {
		return err
	}
	return nil
}

func (r *BesuReconciler) withStandardAnnotations(annotations map[string]string) map[string]string {
	for k, v := range r.config.Besu.Annotations {
		annotations[k] = v
	}
	return annotations
}

func (r *BesuReconciler) generatePDBTemplate(node *corev1alpha1.Besu, name string) *policyv1.PodDisruptionBudget {
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

func (r *BesuReconciler) createPDB(ctx context.Context, node *corev1alpha1.Besu, name string) (*policyv1.PodDisruptionBudget, error) {
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
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionPDB, metav1.ConditionTrue, corev1alpha1.ReasonPDBCreated, fmt.Sprintf("Name: %s", name))
		log.FromContext(ctx).Info("Created Besu pod disruption budget", "Name", name)
	} else if err != nil {
		return nil, err
	}

	return pdb, nil
}

func (r *BesuReconciler) generateStatefulSetTemplate(node *corev1alpha1.Besu, name, configSum string) *appsv1.StatefulSet {
	// Define the StatefulSet to run Besu using the ConfigMap
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: node.Namespace,
			Labels:    r.getLabels(node),
			Annotations: r.withStandardAnnotations(map[string]string{
				"kubectl.kubernetes.io/default-container": "besu",
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
							Name:            "besu",
							Image:           r.config.Besu.Image, // Use the image from the config
							ImagePullPolicy: r.config.Besu.ImagePullPolicy,
							SecurityContext: r.config.Besu.SecurityContext,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/config",
									ReadOnly:  true,
								},
								{
									Name:      "nodeid",
									MountPath: "/nodeid",
									ReadOnly:  true,
								},
								{
									Name:      "genesis",
									MountPath: "/genesis",
									ReadOnly:  true,
								},
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							Args: []string{
								"--config-file",
								"/config/pldconf.besu.toml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "rpc-http",
									ContainerPort: 8545,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "rpc-ws",
									ContainerPort: 8546,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "graphql-http",
									ContainerPort: 8547,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "p2p-tcp",
									ContainerPort: 30303,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "p2p-udp",
									ContainerPort: 30303,
									Protocol:      corev1.ProtocolUDP,
								},
							},
							Env: buildEnv(r.config.Besu.Envs),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"sh",
											"-c",
											"pidof java",
										},
									},
								},
								InitialDelaySeconds: 3,
								TimeoutSeconds:      1,
								PeriodSeconds:       2,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(8545),
									},
								},
								InitialDelaySeconds: 5,
								TimeoutSeconds:      2,
								PeriodSeconds:       5,
							},
						},
					},
					Tolerations:  r.config.Besu.Tolerations,
					Affinity:     r.config.Besu.Affinity,
					NodeSelector: r.config.Besu.NodeSelector,
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
						{
							Name: "nodeid",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: generateBesuIDSecretName(node.Name),
								},
							},
						},
						{
							Name: "genesis",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: generateBesuGenesisName(node.Spec.Genesis),
									},
								},
							},
						},
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: generateBesuPVCName(node.Name),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *BesuReconciler) createService(ctx context.Context, node *corev1alpha1.Besu, name string) (*corev1.Service, error) {
	svc := r.generateServiceTemplate(node, name)
	if err := controllerutil.SetControllerReference(node, svc, r.Scheme); err != nil {
		return nil, err
	}

	var foundSvc corev1.Service
	if err := r.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &foundSvc); err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, svc)
		if err != nil {
			return svc, err
		}
		setCondition(&node.Status.Conditions, corev1alpha1.ConditionSVC, metav1.ConditionTrue, corev1alpha1.ReasonSVCCreated, fmt.Sprintf("Name: %s", name))
		log.FromContext(ctx).Info("Created Besu Service", "Name", name)
	} else if err != nil {
		return svc, err
	}
	return svc, nil
}

// generateServiceTemplate generates a ConfigMap for the Besu configuration
func (r *BesuReconciler) generateServiceTemplate(node *corev1alpha1.Besu, name string) *corev1.Service {
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
			Port:       8545,
			TargetPort: intstr.FromInt(8545),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "rpc-ws",
			Port:       8546,
			TargetPort: intstr.FromInt(8546),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "graphql-http",
			Port:       8547,
			TargetPort: intstr.FromInt(8547),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "p2p-tcp",
			Port:       30303,
			TargetPort: intstr.FromInt(30303),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "p2p-udp",
			Port:       30303,
			TargetPort: intstr.FromInt(30303),
			Protocol:   corev1.ProtocolUDP,
		},
	})
	return svc
}
