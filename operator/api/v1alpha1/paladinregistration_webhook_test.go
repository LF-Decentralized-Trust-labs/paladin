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
package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PaladinRegistration Webhook - Block Update & Delete", func() {

	ctx := context.Background()

	key := types.NamespacedName{
		Name:      "resource",
		Namespace: "default",
	}

	pr := &PaladinRegistration{}

	BeforeEach(func() {
		// Create a valid PaladinRegistration that should pass creation validation
		err := k8sClient.Get(ctx, key, pr)
		if err != nil && errors.IsNotFound(err) {
			pr = &PaladinRegistration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: PaladinRegistrationSpec{
					Registry:          "evm-registry",
					RegistryAdminNode: "node1",
					RegistryAdminKey:  "deployKey",
					Node:              "node1",
					NodeKey:           "registryAdmin",
					Transports:        []string{"grpc"},
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
		}
	})

	Context("When attempting to update an existing PaladinRegistration", func() {
		It("Should block the update and return an error", func() {
			// Fetch the newly created resource to ensure we have the latest version
			fetched := &PaladinRegistration{}
			err := k8sClient.Get(context.Background(), key, fetched)
			Expect(err).NotTo(HaveOccurred())

			fetched.Spec.Registry = "other-registry"

			// Attempt to update, expecting the webhook to block
			By("Updating PaladinRegistration, expecting a webhook error")
			err = k8sClient.Update(context.Background(), fetched)
			Expect(err).To(HaveOccurred())
			// Check the error message from the webhook
			Expect(err.Error()).To(ContainSubstring("updates to PaladinRegistration resources are not allowed"))
		})
	})

	Context("When attempting to delete an existing PaladinRegistration", func() {
		It("Should block the delete and return an error", func() {
			// Attempt to delete, expecting the webhook to block
			By("Deleting PaladinRegistration, expecting a webhook error")
			err := k8sClient.Delete(context.Background(), pr)
			Expect(err).To(HaveOccurred())
			// Check the error message from the webhook
			Expect(err.Error()).To(ContainSubstring("deletions of PaladinRegistration resources are not allowed"))
		})
	})
})
