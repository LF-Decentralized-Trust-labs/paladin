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

var _ = Describe("TransactionInvoke Webhook - Block Update & Delete", func() {

	ctx := context.Background()

	key := types.NamespacedName{
		Name:      "resource",
		Namespace: "default",
	}

	ti := &TransactionInvoke{}

	BeforeEach(func() {
		// Attempt to get the resource. If it doesn't exist, create a valid one.
		err := k8sClient.Get(ctx, key, ti)
		if err != nil && errors.IsNotFound(err) {
			resource := &TransactionInvoke{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: TransactionInvokeSpec{
					TxType: "public",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When attempting to update an existing TransactionInvoke", func() {
		It("Should block the update and return an error", func() {
			fetched := &TransactionInvoke{}
			By("Fetching the TransactionInvoke to ensure we have the latest version")
			err := k8sClient.Get(ctx, key, fetched)
			Expect(err).NotTo(HaveOccurred())

			// Modify a field in the spec to simulate an update
			fetched.Spec.TxType = "private"

			By("Updating TransactionInvoke, expecting a webhook error")
			err = k8sClient.Update(ctx, fetched)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updates to TransactionInvoke resources are not allowed"))
		})
	})

	Context("When attempting to delete an existing TransactionInvoke", func() {
		It("Should block the delete and return an error", func() {
			By("Deleting TransactionInvoke, expecting a webhook error")
			err := k8sClient.Delete(ctx, ti)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deletions of TransactionInvoke resources are not allowed"))
		})
	})
})
