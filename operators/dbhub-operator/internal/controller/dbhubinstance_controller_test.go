/*
Copyright 2026.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbhubv1alpha1 "github.com/Tributary-ai-services/dbhub-operator/api/v1alpha1"
)

var _ = Describe("DBHubInstance Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-dbhub-instance"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		dbhubinstance := &dbhubv1alpha1.DBHubInstance{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DBHubInstance")
			err := k8sClient.Get(ctx, typeNamespacedName, dbhubinstance)
			if err != nil && errors.IsNotFound(err) {
				replicas := int32(1)
				resource := &dbhubv1alpha1.DBHubInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: dbhubv1alpha1.DBHubInstanceSpec{
						Replicas:  &replicas,
						Image:     "bytebase/dbhub:latest",
						Transport: dbhubv1alpha1.TransportTypeHTTP,
						Port:      8080,
						DatabaseSelector: &dbhubv1alpha1.DatabaseSelector{
							MatchLabels: map[string]string{
								"environment": "test",
							},
						},
						DefaultPolicy: &dbhubv1alpha1.DefaultPolicy{
							ReadOnly: true,
							MaxRows:  1000,
							AllowedOperations: []string{
								"execute_sql",
								"search_objects",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the DBHubInstance resource
			resource := &dbhubv1alpha1.DBHubInstance{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance DBHubInstance")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DBHubInstanceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the resource status was updated
			By("Checking the resource status")
			updatedInstance := &dbhubv1alpha1.DBHubInstance{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedInstance)).To(Succeed())
			// With no matching databases, the instance should still create resources
			Expect(updatedInstance.Status.Endpoint).To(ContainSubstring("test-dbhub-instance"))
		})
	})
})
