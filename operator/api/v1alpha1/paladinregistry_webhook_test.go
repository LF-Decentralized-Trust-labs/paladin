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

var _ = Describe("PaladinRegistry Webhook - Block Update & Delete", func() {

	ctx := context.Background()

	key := types.NamespacedName{
		Name:      "resource",
		Namespace: "default",
	}

	registry := &PaladinRegistry{}

	BeforeEach(func() {
		// Attempt to fetch the resource. If it doesn't exist, create a valid one.
		err := k8sClient.Get(ctx, key, registry)
		if err != nil && errors.IsNotFound(err) {
			registry = &PaladinRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: PaladinRegistrySpec{
					Type: RegistryTypeEVM,
					EVM: EVMRegistryConfig{
						SmartContractDeployment: "registry",
					},
					Plugin: PluginConfig{
						Type:    "c-shared",
						Library: "/app/registries/libevm.so",
					},
					ConfigJSON: "{}",
				},
			}
			Expect(k8sClient.Create(ctx, registry)).To(Succeed())
		} else {
			// If any other error occurred, fail the test immediately
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When attempting to update an existing PaladinRegistry", func() {
		It("Should block the update and return an error", func() {
			fetched := &PaladinRegistry{}
			// Fetch the resource to ensure we have the latest version
			err := k8sClient.Get(ctx, key, fetched)
			Expect(err).NotTo(HaveOccurred())

			// Attempt to change something in the spec
			fetched.Spec.ConfigJSON = "{\"some\":\"update\"}"

			// Update should fail due to the webhook blocking updates
			By("Updating PaladinRegistry, expecting a webhook error")
			err = k8sClient.Update(ctx, fetched)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updates to PaladinRegistry resources are not allowed"))
		})
	})

	Context("When attempting to delete an existing PaladinRegistry", func() {
		It("Should block the delete and return an error", func() {
			// Attempt to delete, expecting the webhook to block
			By("Deleting PaladinRegistry, expecting a webhook error")
			err := k8sClient.Delete(ctx, registry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deletions of PaladinRegistry resources are not allowed"))
		})
	})
})
