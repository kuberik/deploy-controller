package live

import (
	"testing"

	kuberikv1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/resmap"
	resmaptest_test "sigs.k8s.io/kustomize/api/testutils/resmaptest"
)

func TestNewLiveApply(t *testing.T) {
	testCases := []struct {
		name      string
		live      *kuberikv1alpha1.Live
		resources resmap.ResMap
		wantErr   bool
	}{{
		name: "extra-resource-group",
		live: &kuberikv1alpha1.Live{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		},
		wantErr: true,
		resources: resmaptest_test.NewRmBuilder(t, rf).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "reconcile",
					"namespace": "default",
				}}).
			Add(map[string]interface{}{
				"apiVersion": "kpt.dev/v1alpha1",
				"kind":       "ResourceGroup",
				"metadata": map[string]interface{}{
					"name":      "reconcile",
					"namespace": "default",
				}}).ResMap(),
	}, {
		name: "normal",
		live: &kuberikv1alpha1.Live{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
				UID:       "test",
			},
		},
		resources: resmaptest_test.NewRmBuilder(t, rf).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "foo",
					"namespace": "bar",
				}}).ResMap(),
	}, {
		name: "some-extra-resources",
		live: &kuberikv1alpha1.Live{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "root",
				Namespace: "kube-system",
				UID:       "test",
			},
		},
		resources: resmaptest_test.NewRmBuilder(t, rf).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "root",
					"namespace": "kube-system",
				}}).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "cm1",
				}}).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "cm2",
				}}).
			Add(map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "cm3",
				}}).ResMap(),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			liveApply, err := NewLiveApply(tc.live, tc.resources)
			if tc.wantErr {
				assert.Assert(t, err != nil, "expected error but got nil")
				assert.Assert(t, liveApply == nil, "expected nil liveApply but got %v", liveApply)
			} else {
				assert.NilError(t, err, "failed to create live deployment")
				assert.Assert(t, liveApply != nil, "expected liveApply but got nil")

				gotDeployResourcesCount := len(liveApply.Resources())
				wantDeployResourcesCount := len(tc.resources.Resources()) + 1
				assert.Assert(t, gotDeployResourcesCount == wantDeployResourcesCount, "want %d resources but got %d", wantDeployResourcesCount, gotDeployResourcesCount)

				resourceGroupPresent := false
				for _, r := range liveApply.Resources() {
					if r.GetApiVersion() == "kpt.dev/v1alpha1" && r.GetKind() == "ResourceGroup" && r.GetName() == tc.live.Name && r.GetNamespace() == tc.live.Namespace {
						resourceGroupPresent = true
						inventoryID := r.GetLabels()["cli-utils.sigs.k8s.io/inventory-id"]
						assert.Assert(t, inventoryID == string(tc.live.UID), "expected resource group inventory-id %s but got %s", tc.live.UID, inventoryID)
						break
					}
				}
				assert.Assert(t, resourceGroupPresent, "expected ResourceGroup but got none")
			}
		})
	}
}
