// Copyright 2024-2025 Andres Morey
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/kubetail-org/kubetail/modules/shared/grpchelpers"
)

func allowSSAR(clientset *fake.Clientset, namespaces []string, verbs []string) {
	clientset.Fake.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		// Cast the action to CreateAction to access the object being created
		createAction := action.(k8stesting.CreateAction)
		ssar := createAction.GetObject().(*authv1.SelfSubjectAccessReview)

		// check ssar
		if slices.Contains(verbs, ssar.Spec.ResourceAttributes.Verb) && slices.Contains(namespaces, ssar.Spec.ResourceAttributes.Namespace) {
			ssar.Status.Allowed = true
		}

		// Return the modified SelfSubjectAccessReview
		return true, ssar, nil
	})
}

func TestCheckPermissionFailure(t *testing.T) {
	ctxWithToken := context.WithValue(context.Background(), grpchelpers.K8STokenCtxKey, "xxx")

	t.Run("missing token", func(t *testing.T) {
		err := CheckPermission(context.Background(), nil, []string{}, "x")
		require.ErrorContains(t, err, "missing token")
	})

	t.Run("namespaces required", func(t *testing.T) {
		err := CheckPermission(ctxWithToken, nil, []string{}, "x")
		require.ErrorContains(t, err, "namespaces required")
	})

	t.Run("single namespace", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		err := CheckPermission(ctxWithToken, clientset, []string{"ns1"}, "x")
		require.ErrorContains(t, err, "permission denied")
	})

	t.Run("multiple namespaces", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		err := CheckPermission(ctxWithToken, clientset, []string{"ns1", "ns2"}, "x")
		require.ErrorContains(t, err, "permission denied")
	})

	t.Run("one of several not allowed", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		allowSSAR(clientset, []string{"ns1"}, []string{"x"})
		err := CheckPermission(ctxWithToken, clientset, []string{"ns1", "ns2"}, "x")
		require.ErrorContains(t, err, "permission denied")
	})
}

func TestCheckPermissionSuccess(t *testing.T) {
	ctxWithToken := context.WithValue(context.Background(), grpchelpers.K8STokenCtxKey, "xxx")

	t.Run("single namespace", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		allowSSAR(clientset, []string{"ns1"}, []string{"x"})
		err := CheckPermission(ctxWithToken, clientset, []string{"ns1"}, "x")
		require.Nil(t, err)
	})

	t.Run("multiple namespaces", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		allowSSAR(clientset, []string{"ns1", "ns2"}, []string{"x"})
		err := CheckPermission(ctxWithToken, clientset, []string{"ns1", "ns2"}, "x")
		require.Nil(t, err)
	})
}

func TestCheckPermissionRequest(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	clientset.Fake.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		// Cast the action to CreateAction to access the object being created
		createAction := action.(k8stesting.CreateAction)
		ssar := createAction.GetObject().(*authv1.SelfSubjectAccessReview)

		// check ssar
		require.Equal(t, "ns1", ssar.Spec.ResourceAttributes.Namespace)
		require.Equal(t, "", ssar.Spec.ResourceAttributes.Group)
		require.Equal(t, "x", ssar.Spec.ResourceAttributes.Verb)
		require.Equal(t, "pods/log", ssar.Spec.ResourceAttributes.Resource)

		// Set the Allowed field to true in the response
		ssar.Status.Allowed = true

		// Return the modified SelfSubjectAccessReview
		return true, ssar, nil
	})

	CheckPermission(context.Background(), clientset, []string{"ns1"}, "x")
}
