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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbhubv1alpha1 "github.com/Tributary-ai-services/dbhub-operator/api/v1alpha1"
)

var _ = Describe("Database Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-database"
		const secretName = "test-database-secret"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		database := &dbhubv1alpha1.Database{}

		BeforeEach(func() {
			// Create a secret for database credentials
			By("creating the credentials secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &corev1.Secret{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			By("creating the custom resource for the Kind Database")
			err = k8sClient.Get(ctx, typeNamespacedName, database)
			if err != nil && errors.IsNotFound(err) {
				resource := &dbhubv1alpha1.Database{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: dbhubv1alpha1.DatabaseSpec{
						Type:     dbhubv1alpha1.DatabaseTypePostgres,
						Host:     "localhost",
						Port:     5432,
						Database: "testdb",
						CredentialsRef: dbhubv1alpha1.CredentialsRef{
							Name:        secretName,
							UserKey:     "username",
							PasswordKey: "password",
						},
						SSLMode:           dbhubv1alpha1.SSLModeDisable,
						ConnectionTimeout: 30,
						QueryTimeout:      60,
						ReadOnly:          true,
						MaxRows:           1000,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup the Database resource
			resource := &dbhubv1alpha1.Database{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Database")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Cleanup the Secret
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret)
			if err == nil {
				By("Cleanup the credentials secret")
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// The reconciler will fail to connect since there's no real database,
			// but it should not return an error - it should update the status
			Expect(err).NotTo(HaveOccurred())

			// Verify the resource status was updated
			By("Checking the resource status")
			updatedDatabase := &dbhubv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Status.Phase).To(Equal(dbhubv1alpha1.DatabasePhaseFailed))
			Expect(updatedDatabase.Status.DSN).To(ContainSubstring("postgres://localhost:5432/testdb"))
		})
	})
})
