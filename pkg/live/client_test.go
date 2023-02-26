package live

import (
	"context"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/kustomize/api/resmap"
	resmaptest_test "sigs.k8s.io/kustomize/api/testutils/resmaptest"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func initEnvTest(t *testing.T) *rest.Config {
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	assert.NilError(t, err, "failed to start test environment")
	t.Cleanup(func() { testEnv.Stop() })
	return cfg
}

func TestKptkptClientApplyWaitForReconcile(t *testing.T) {
	build := resmaptest_test.NewRmBuilder(t, rf).
		Add(map[string]interface{}{
			"apiVersion": "kpt.dev/v1alpha1",
			"kind":       "ResourceGroup",
			"metadata": map[string]interface{}{
				"name":      "reconcile",
				"namespace": "default",
				"labels": map[string]interface{}{
					"cli-utils.sigs.k8s.io/inventory-id": "reconcile-id",
				},
			}}).
		Add(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "nginx",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"containers": []map[string]interface{}{{
					"name":  "nginx",
					"image": "nginx",
				}},
			}}).
		Add(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "nginx",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"ports": []map[string]interface{}{{
					"port":       8080,
					"targetPort": 8080,
				}},
			}}).ResMap()

	cfg := initEnvTest(t)

	kptClient, err := NewKptClient(context.TODO(), *cfg)
	assert.NilError(t, err, "failed to create kptClient")
	err = kptClient.ImpersonateForResources(types.NamespacedName{Namespace: "default", Name: "apply-wait-reconcile"})
	assert.NilError(t, err, "failed to set impersonation for kptClient")

	err = kptClient.InstallResourceGroup()
	assert.NilError(t, err, "failed to install resource group")

	applied := make(chan error)
	go func() {
		applied <- kptClient.Apply(build, ApplyOptions{})
	}()

	clientset, err := kubernetes.NewForConfig(cfg)
	assert.NilError(t, err, "failed to create clientset")

	_, err = clientset.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apply-wait-reconcile",
			Namespace: "default",
		},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	_, err = clientset.RbacV1().Roles("default").Create(context.TODO(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apply-wait-reconcile",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"services", "pods"},
			Verbs:     []string{"*"},
		}},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	_, err = clientset.RbacV1().RoleBindings("default").Create(context.TODO(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apply-wait-reconcile",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			Name: "apply-wait-reconcile",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{{
			Name:      "apply-wait-reconcile",
			Namespace: "default",
			Kind:      rbacv1.ServiceAccountKind,
		}},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	reconciled := false
	nginxPodAPI := clientset.CoreV1().Pods("default")
	select {
	case <-applied:
		if !reconciled {
			t.Fatalf("apply shouldn't have reconciled yet")
		}
	case <-time.After(3 * time.Second):
		_, err := nginxPodAPI.Get(context.TODO(), "nginx", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get nginx pod: %v", err)
		}
		reconciled = true
		nginxPod, err := nginxPodAPI.Get(context.TODO(), "nginx", metav1.GetOptions{})
		assert.NilError(t, err, "failed to get nginx pod")
		nginxPod.Status.Phase = corev1.PodSucceeded
		nginxPodAPI.UpdateStatus(context.TODO(), nginxPod, metav1.UpdateOptions{})
	}
	err = <-applied
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	service, err := clientset.CoreV1().Services("default").Get(context.TODO(), "nginx", metav1.GetOptions{})
	assert.NilError(t, err, "failed to get applied service")
	service.ManagedFields[0].Time = nil
	assert.DeepEqual(t, service.ManagedFields, []metav1.ManagedFieldsEntry{{
		Manager:    "rg/default/reconcile",
		Operation:  "Apply",
		APIVersion: "v1",
		FieldsType: "FieldsV1",
		FieldsV1: &metav1.FieldsV1{
			Raw: []byte(`{"f:metadata":{"f:annotations":{"f:config.k8s.io/owning-inventory":{}}},"f:spec":{"f:ports":{"k:{\"port\":8080,\"protocol\":\"TCP\"}":{".":{},"f:port":{},"f:targetPort":{}}}}}`),
		},
	}})

	deletedResourcesBuild := resmap.New()
	for _, r := range build.Resources()[:1] {
		deletedResourcesBuild.Append(r)
	}

	err = kptClient.Apply(deletedResourcesBuild, ApplyOptions{})
	assert.NilError(t, err, "failed to apply")
	_, err = nginxPodAPI.Get(context.TODO(), "nginx", metav1.GetOptions{})
	assert.Assert(t, errors.IsNotFound(err), "nginx pod should have been deleted, %s", err)
}

func TestKptkptClientDestroyWaitForReconcile(t *testing.T) {
	build := resmaptest_test.NewRmBuilder(t, rf).
		Add(map[string]interface{}{
			"apiVersion": "kpt.dev/v1alpha1",
			"kind":       "ResourceGroup",
			"metadata": map[string]interface{}{
				"name":      "destroy",
				"namespace": "default",
				"labels": map[string]interface{}{
					"cli-utils.sigs.k8s.io/inventory-id": "destroy-id",
				},
			}}).
		Add(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "foo-destroy",
				"namespace": "default",
				"finalizers": []string{
					"kuberik.io/dummy",
				},
			}}).ResMap()

	cfg := initEnvTest(t)

	clientset, err := kubernetes.NewForConfig(cfg)
	assert.NilError(t, err, "failed to create clientset")

	_, err = clientset.CoreV1().ServiceAccounts("default").Create(context.TODO(), &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "destroy-wait-reconcile",
			Namespace: "default",
		},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	_, err = clientset.RbacV1().Roles("default").Create(context.TODO(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "destroy-wait-reconcile",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"*"},
		}},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	_, err = clientset.RbacV1().RoleBindings("default").Create(context.TODO(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "destroy-wait-reconcile",
			Namespace: "default",
		},
		RoleRef: rbacv1.RoleRef{
			Name: "destroy-wait-reconcile",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{{
			Name:      "destroy-wait-reconcile",
			Namespace: "default",
			Kind:      rbacv1.ServiceAccountKind,
		}},
	}, metav1.CreateOptions{})
	assert.NilError(t, err)

	kptClient, err := NewKptClient(context.TODO(), *cfg)
	assert.NilError(t, err, "failed to create kptClient")
	err = kptClient.ImpersonateForResources(types.NamespacedName{Namespace: "default", Name: "destroy-wait-reconcile"})
	assert.NilError(t, err, "failed to set impersonation for kptClient")
	assert.NilError(t, kptClient.InstallResourceGroup(), "failed to install resource group")
	assert.NilError(t, kptClient.Apply(build, ApplyOptions{}), "failed to apply resources")

	deleted := make(chan error)
	go func() {
		deleted <- kptClient.Destroy(types.NamespacedName{Name: "destroy", Namespace: "default"}, "destroy-id")
	}()

	reconciled := false
	configMapAPI := clientset.CoreV1().ConfigMaps("default")
	select {
	case <-deleted:
		if !reconciled {
			t.Fatalf("destroy shouldn't have finished yet")
		}
	case <-time.After(3 * time.Second):
		_, err := configMapAPI.Get(context.TODO(), "foo-destroy", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get configmap: %v", err)
		}
		reconciled = true
		cm, err := configMapAPI.Get(context.TODO(), "foo-destroy", metav1.GetOptions{})
		assert.NilError(t, err, "failed to get foo-destroy pod")

		cm.Finalizers = []string{}
		configMapAPI.Update(context.TODO(), cm, metav1.UpdateOptions{})
	}
	err = <-deleted
	if err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	_, err = configMapAPI.Get(context.TODO(), "foo-destroy", metav1.GetOptions{})
	assert.Assert(t, errors.IsNotFound(err), "foo-destroy configmap should have been deleted, %s", err)
}

func TestKptkptClientApplyForbidden(t *testing.T) {
	build := resmaptest_test.NewRmBuilder(t, rf).
		Add(map[string]interface{}{
			"apiVersion": "kpt.dev/v1alpha1",
			"kind":       "ResourceGroup",
			"metadata": map[string]interface{}{
				"name":      "forbidden",
				"namespace": "default",
				"labels": map[string]interface{}{
					"cli-utils.sigs.k8s.io/inventory-id": "forbidden-id",
				},
			}}).
		Add(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "forbidden",
				"namespace": "default",
			}}).ResMap()

	cfg := initEnvTest(t)

	kptClient, err := NewKptClient(context.TODO(), *cfg)
	assert.NilError(t, err, "failed to create kptClient")
	err = kptClient.ImpersonateForResources(types.NamespacedName{Namespace: "default", Name: "apply-wait-reconcile"})
	assert.NilError(t, err, "failed to set impersonation for kptClient")

	err = kptClient.InstallResourceGroup()
	assert.NilError(t, err, "failed to install resource group")

	err = kptClient.Apply(build, ApplyOptions{})
	assert.Assert(t, strings.Contains(err.Error(), "forbidden"))

	clientset, err := kubernetes.NewForConfig(cfg)
	assert.NilError(t, err, "failed to create clientset")

	_, err = clientset.CoreV1().ConfigMaps("default").Get(context.TODO(), "forbidden", metav1.GetOptions{})
	assert.Assert(t, errors.IsNotFound(err), "forbidden configmap should not have been created, %s", err)
}
