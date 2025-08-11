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
	"encoding/json"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	"github.com/LF-Decentralized-Trust-labs/paladin/testinfra/pkg/besugenesis"
	"github.com/hyperledger/firefly-signer/pkg/ethtypes"
)

// BesuGenesisReconciler reconciles a BesuGenesis object
type BesuGenesisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// allows generic functions by giving a mapping between the types and interfaces for the CR
var BesuGenesisCRMap = CRMap[corev1alpha1.BesuGenesis, *corev1alpha1.BesuGenesis, *corev1alpha1.BesuGenesisList]{
	NewList:  func() *corev1alpha1.BesuGenesisList { return new(corev1alpha1.BesuGenesisList) },
	ItemsFor: func(list *corev1alpha1.BesuGenesisList) []corev1alpha1.BesuGenesis { return list.Items },
	AsObject: func(item *corev1alpha1.BesuGenesis) *corev1alpha1.BesuGenesis { return item },
}

// Reconcile implements the logic when a BesuGenesis resource is created, updated, or deleted
func (r *BesuGenesisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the BesuGenesis instance
	var genesis corev1alpha1.BesuGenesis
	if err := r.Get(ctx, req.NamespacedName, &genesis); err != nil {
		if errors.IsNotFound(err) {
			log.Info("BesuGenesis resource deleted, deleting related resources")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get BesuGenesis resource")
		return ctrl.Result{}, err
	}

	// Initialize status if empty
	if genesis.Status.Phase == "" {
		genesis.Status.Phase = corev1alpha1.StatusPhaseFailed
	}

	defer func() {
		// Update the overall phase based on conditions
		if err := r.Status().Update(ctx, &genesis); err != nil {
			log.Error(err, "Failed to update Besu status")
		}
	}()

	// Build the genesis file (depends on the creation of the identities of all the nodes)
	_, ready, err := r.createConfigMap(ctx, &genesis)
	if err != nil {
		log.Error(err, "Failed to create BesuGenesis config map")
		setCondition(&genesis.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionFalse, corev1alpha1.ReasonCMCreationFailed, err.Error())
		return ctrl.Result{}, err
	} else if !ready {
		log.Info("Waiting for node identities before creating BesuGenesis config map")
		return ctrl.Result{
			// Keep polling on a short duration until we get the identities
			RequeueAfter: time.Duration(1),
		}, nil

	}
	genesis.Status.Phase = corev1alpha1.StatusPhaseReady

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BesuGenesisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.BesuGenesis{}).
		// Reconcile when any besu status changes
		Watches(&corev1alpha1.Besu{}, reconcileAll(BesuGenesisCRMap, r.Client), reconcileEveryChange()).
		Complete(r)
}

func (r *BesuGenesisReconciler) createConfigMap(ctx context.Context, genesis *corev1alpha1.BesuGenesis) (*corev1.ConfigMap, bool, error) {
	var genesisMap corev1.ConfigMap
	name := generateBesuGenesisName(genesis.Name)
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: genesis.Namespace}, &genesisMap); err != nil && errors.IsNotFound(err) {
		// No existing map - create a new genesis map.
		// Process should be deterministic - but as it's final we only do it when configmap does not exist
		newMap, ready, err := r.generateNewGenesisMap(ctx, genesis, name)
		if err != nil || !ready {
			return nil, ready, err
		}
		if err := controllerutil.SetControllerReference(genesis, newMap, r.Scheme); err != nil {
			return nil, false, err
		}

		err = r.Create(ctx, newMap)
		if err != nil {
			return nil, false, err
		}
		setCondition(&genesis.Status.Conditions, corev1alpha1.ConditionCM, metav1.ConditionTrue, corev1alpha1.ReasonCMCreated, fmt.Sprintf("Name: %s", name))
		return newMap, true, nil
	} else if err != nil {
		return nil, false, err
	}
	return &genesisMap, true, nil
}

func (r *BesuGenesisReconciler) generateNewGenesisMap(ctx context.Context, genesis *corev1alpha1.BesuGenesis, name string) (*corev1.ConfigMap, bool, error) {
	var g besugenesis.GenesisJSON
	if genesis.Spec.Base != "" {
		if err := json.Unmarshal([]byte(genesis.Spec.Base), &g); err != nil {
			return nil, false, fmt.Errorf("supplied base genesis JSON could not be parsed: %s", err)
		}
	}

	// We always set the chain ID
	g.Config.ChainID = int64(genesis.Spec.ChainID)
	// We always set the gas limit
	g.GasLimit = ethtypes.HexUint64(genesis.Spec.GasLimit)

	// Get the validator IDs
	validatorAddresses, ready, err := r.getInitialValidators(ctx, genesis)
	if !ready || err != nil {
		return nil, ready, err
	}

	// Lots of detail around how we set up QBFT
	if err := r.setQBFTConfig(validatorAddresses, genesis, &g); err != nil {
		return nil, false, err
	}

	// Remaining fields we provide defaults for, which are fixed based on the Besu doc examples.
	// CONSTANTS CANNOT CHANGE WITHOUT AFFECTING RE-GENERATION OF EXISTING GENESIS CONFIGMAPS

	if g.Config.ZeroBaseFee == nil {
		g.Config.ZeroBaseFee = ptrTo(true)
	}
	if g.Timestamp == 0 {
		g.Timestamp = ethtypes.HexUint64(0x5b3d92d7)
	}
	if g.MixHash == nil {
		g.MixHash = ethtypes.MustNewHexBytes0xPrefix("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")
	}
	if g.Difficulty == 0 {
		g.Difficulty = 1
	}
	if g.Coinbase == nil {
		g.Coinbase = ethtypes.MustNewAddress("0x0000000000000000000000000000000000000000")
	}
	if g.Alloc == nil {
		g.Alloc = map[string]besugenesis.AllocEntry{}
	}

	genesisJSON, err := json.MarshalIndent(&g, "", "  ")
	if err != nil {
		return nil, false, err
	}

	return r.wrapGenesisInConfigMap(genesis, string(genesisJSON), name), true, nil
}

func (r *BesuGenesisReconciler) loadInitialValidatorIDSecrets(ctx context.Context, namespace string, names []string) ([]corev1.Secret, error) {
	// We use a label to mark all the secrets that should be node ID
	isBesuNodeIDForValidator, _ := labels.NewRequirement("besu-node-id", selection.In, names)
	var secrets corev1.SecretList
	if err := r.List(ctx, &secrets, client.InNamespace(namespace), client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*isBesuNodeIDForValidator),
	}); err != nil {
		return nil, err
	}
	return secrets.Items, nil
}

func (r *BesuGenesisReconciler) getInitialValidators(ctx context.Context, genesis *corev1alpha1.BesuGenesis) ([]ethtypes.Address0xHex, bool, error) {
	if len(genesis.Spec.InitialValidators) == 0 {
		return nil, false, fmt.Errorf("at least one initial validator must be provided")
	}

	secrets, err := r.loadInitialValidatorIDSecrets(ctx, genesis.Namespace, genesis.Spec.InitialValidators)
	if err != nil {
		return nil, false, err
	}

	// Check all the nodes we need have written their identities
	if len(secrets) != len(genesis.Spec.InitialValidators) {
		log := log.FromContext(ctx)
		log.Info(fmt.Sprintf("Found identities %d of %d initial validator nodes", len(secrets), len(genesis.Spec.InitialValidators)))
		return nil, false, nil // not ready yet
	}

	// Calculate all the addresses
	addrs := make([]ethtypes.Address0xHex, len(genesis.Spec.InitialValidators))
	for i, validatorName := range genesis.Spec.InitialValidators {
		var secret *corev1.Secret
		for _, possible := range secrets {
			if possible.Labels["besu-node-id"] == validatorName {
				tmpPossible := possible
				secret = &tmpPossible
				break
			}
		}
		if secret == nil || secret.Data["address"] == nil {
			return nil, false, fmt.Errorf("failed to resole validator %s from returned config map list", validatorName)
		}
		addr, err := ethtypes.NewAddress(string(secret.Data["address"]))
		if err != nil {
			return nil, false, fmt.Errorf("invalid address in identity secret '%s'", secret.Name)
		}
		addrs[i] = *addr
	}
	return addrs, true, nil

}

func (r *BesuGenesisReconciler) setQBFTConfig(validatorAddresses []ethtypes.Address0xHex, genesis *corev1alpha1.BesuGenesis, g *besugenesis.GenesisJSON) error {
	// Currently only support QBFT consensus
	qbftConfig := g.Config.QBFT
	if qbftConfig == nil {
		qbftConfig = &besugenesis.QBFTConfig{}
		g.Config.QBFT = qbftConfig
	}

	// We always set the block period
	blockPeriodDuration, err := time.ParseDuration(genesis.Spec.BlockPeriod)
	if err != nil {
		return fmt.Errorf("invalid blockPeriod: %s", err)
	}
	if blockPeriodDuration < 1*time.Second {
		qbftConfig.BlockPeriodSeconds = ptrTo(1) // will be ignored on Besu where millis are supported
		qbftConfig.BlockPeriodMilliseconds = ptrTo(int(blockPeriodDuration.Milliseconds()))
	} else {
		qbftConfig.BlockPeriodSeconds = ptrTo(nearestIntegerAboveZero(blockPeriodDuration.Seconds()))
		qbftConfig.BlockPeriodMilliseconds = nil
	}
	if genesis.Spec.BlockPeriod != "" {
		emptyBlockPeriodDuration, err := time.ParseDuration(genesis.Spec.EmptyBlockPeriod)
		if err != nil {
			return fmt.Errorf("invalid blockPeriod: %s", err)
		}
		if emptyBlockPeriodDuration > 0 {
			qbftConfig.EmptyBlockPeriodSeconds = ptrTo(nearestIntegerAboveZero(emptyBlockPeriodDuration.Seconds()))
		}
	}
	if qbftConfig.RequestTimeoutSeconds == nil {
		qbftConfig.RequestTimeoutSeconds = ptrTo(10 /* constant cannot be changed without affecting existing genesis files */)
	}
	if qbftConfig.EpochLength == nil {
		qbftConfig.EpochLength = ptrTo(30000 /* constant cannot be changed without affecting existing genesis files */)
	}

	// Generate the extra data from the validator list
	g.ExtraData = besugenesis.BuildQBFTExtraData(validatorAddresses...)

	return nil
}

func (r *BesuGenesisReconciler) wrapGenesisInConfigMap(genesis *corev1alpha1.BesuGenesis, genesisJSON string, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: genesis.Namespace,
			Labels: map[string]string{
				"besu-genesis": genesis.Name,
			},
		},
		Data: map[string]string{
			"genesis.json": genesisJSON,
		},
	}
}

// generateBesuName generates a name for the Besu resources based on the Besu name.
// this is for generating unique names for the resources
func generateBesuGenesisName(n string) string {
	return fmt.Sprintf("besu-%s-genesis", n)
}

func nearestIntegerAboveZero(v float64) int {
	return (int)(math.Max(1, math.Round(v)))
}

func ptrTo[T any](v T) *T {
	return &v
}
