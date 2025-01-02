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

var _ = Describe("SmartContractDeployment Webhook - Block Update & Delete", func() {

	ctx := context.Background()

	key := types.NamespacedName{
		Name:      "resource",
		Namespace: "default",
	}

	scd := &SmartContractDeployment{}

	BeforeEach(func() {
		// Attempt to fetch the resource. If it doesn't exist, create a valid one.
		err := k8sClient.Get(ctx, key, scd)
		if err != nil && errors.IsNotFound(err) {
			scd = &SmartContractDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: SmartContractDeploymentSpec{
					TxType: "public",
				},
			}
			Expect(k8sClient.Create(ctx, scd)).To(Succeed())
		} else {
			// If there's a real error, fail
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When attempting to update an existing SmartContractDeployment", func() {
		It("Should block the update and return an error", func() {
			fetched := &SmartContractDeployment{}
			By("Fetching the resource to ensure we have the latest version")
			err := k8sClient.Get(ctx, key, fetched)
			Expect(err).NotTo(HaveOccurred())

			// Make some change to the spec
			fetched.Spec.TxType = "private"

			By("Attempting to update SmartContractDeployment, expecting the webhook to block")
			err = k8sClient.Update(ctx, fetched)
			Expect(err).To(HaveOccurred())
			// Check for the expected error message from the webhook
			Expect(err.Error()).To(ContainSubstring("updates to SmartContractDeployment resources are not allowed"))
		})
	})

	Context("When attempting to delete an existing SmartContractDeployment", func() {
		It("Should block the delete and return an error", func() {
			By("Deleting SmartContractDeployment, expecting a webhook error")
			err := k8sClient.Delete(ctx, scd)
			Expect(err).To(HaveOccurred())
			// Check for the expected error message
			Expect(err.Error()).To(ContainSubstring("deletions of SmartContractDeployment resources are not allowed"))
		})
	})
})
