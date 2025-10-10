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
	"reflect"
	"sort"

	corev1alpha1 "github.com/LF-Decentralized-Trust-labs/paladin/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This mapping object for each CR type between the CR, pointer to the CR, and
// list of items for the CR, lets us build generic functions.
type CRMap[CR any, PCR client.Object, CRL client.ObjectList] struct {
	NewList  func() CRL
	ItemsFor func(list CRL) []CR
	AsObject func(item *CR) PCR
}

func mergeServicePorts(svcSpec *corev1.ServiceSpec, requiredPorts []corev1.ServicePort) {
	portsByName := map[string]*corev1.ServicePort{}
	for _, providedPort := range svcSpec.Ports {
		tmpPort := providedPort
		portsByName[providedPort.Name] = &tmpPort
	}
	for _, requiredPort := range requiredPorts {
		providedPort, isProvided := portsByName[requiredPort.Name]
		if !isProvided {
			// Just use our definition
			tmpPort := requiredPort
			portsByName[requiredPort.Name] = &tmpPort
		} else {
			// We own the target port number and protocol always
			providedPort.TargetPort = requiredPort.TargetPort
			providedPort.Protocol = requiredPort.Protocol
			// Port can be overridden
			if providedPort.Port == 0 {
				providedPort.Port = requiredPort.Port
			}
		}
	}
	portNames := make([]string, 0, len(portsByName))
	for portName := range portsByName {
		portNames = append(portNames, portName)
	}
	// Need to sort by name to give deterministic behavior
	sort.Strings(portNames)
	svcSpec.Ports = make([]corev1.ServicePort, len(portNames))
	for i, portName := range portNames {
		svcSpec.Ports[i] = *portsByName[portName]
	}
}

func reconcileEveryChange() builder.Predicates {
	return builder.WithPredicates(predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return true
		},
	})
}

// This helper reconciles all objects in the current namespace
// The generics are horrible - but the k8s for the interface relations make it tough to re-use
func reconcileAll[CR any, PCR client.Object, CRL client.ObjectList](lf CRMap[CR, PCR, CRL], c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		ns := object.GetNamespace()

		l := lf.NewList()
		err := c.List(ctx, l, client.InNamespace(ns))
		if err != nil {
			return nil
		}

		items := lf.ItemsFor(l)
		if len(items) < 1 {
			return nil
		}
		requests := make([]reconcile.Request, len(items))

		for i, item := range items {
			obj := lf.AsObject(&item)
			requests[i] = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      obj.GetName(),
					Namespace: obj.GetNamespace(),
				},
			}
		}
		return requests
	})
}

// Helpful function if you're running label selectors and generating config, so you need to avoid
// duplicates and also thrashing servers by non-deterministically generating config files.
func deDupAndSortInLocalNS[CR any, PCR client.Object, CRL client.ObjectList](lf CRMap[CR, PCR, CRL], unsorted CRL) []*CR {

	unsortedList := lf.ItemsFor(unsorted)
	uniqueEntries := make(map[string]*CR, len(unsortedList))
	uniqueNames := make([]string, 0, len(unsortedList))
	for _, e := range unsortedList {
		localName := lf.AsObject(&e).GetName()
		if _, isDup := uniqueEntries[localName]; !isDup {
			uniqueNames = append(uniqueNames, localName)
			t := e
			uniqueEntries[localName] = &t
		}
	}
	sort.Strings(uniqueNames)
	sorted := make([]*CR, len(uniqueNames))
	for i, localName := range uniqueNames {
		sorted[i] = uniqueEntries[localName]
	}
	return sorted

}

func setCondition(
	conditions *[]metav1.Condition,
	conditionType corev1alpha1.ConditionType,
	status metav1.ConditionStatus,
	reason corev1alpha1.ConditionReason,
	message string,
) {
	condition := metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             string(reason),
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Update or append the condition
	meta.SetStatusCondition(conditions, condition)
}

func mapToStruct(data map[string][]byte, result interface{}) error {
	// Ensure that result is a pointer to a struct
	v := reflect.ValueOf(result)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("result argument must be a non-nil pointer to a struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("result argument must be a pointer to a struct")
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fieldValue := v.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields
		if !fieldValue.CanSet() {
			continue
		}

		// Get the JSON tag or use the field name
		tag := fieldType.Tag.Get("json")
		if tag == "" {
			tag = fieldType.Name
		}

		// Check if the map contains the key
		if val, exists := data[tag]; exists {
			switch fieldValue.Kind() {
			case reflect.String:
				fieldValue.SetString(string(val))
			default:
				return fmt.Errorf("unsupported field type: %s", fieldValue.Kind())
			}
		}
	}

	return nil
}
