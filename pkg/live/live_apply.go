package live

import (
	"fmt"

	resourcegroupv1alpha1 "github.com/GoogleContainerTools/kpt/pkg/api/resourcegroup/v1alpha1"
	kuberikv1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/resid"
)

type LiveApply struct {
	resmap.ResMap
}

func generateResourceGroup(live *kuberikv1alpha1.Live) (*resource.Resource, error) {
	name := live.GetName()
	if name == "" {
		return nil, fmt.Errorf("live resource must have a name")
	}
	namespace := live.GetNamespace()
	if namespace == "" {
		return nil, fmt.Errorf("live resource must have a namespace")
	}

	var depProvider = provider.NewDefaultDepProvider()
	var rf = depProvider.GetResourceFactory()
	return rf.FromMap(map[string]interface{}{
		"apiVersion": fmt.Sprintf("%s/%s", resourcegroupv1alpha1.RGFileGroup, resourcegroupv1alpha1.RGFileVersion),
		"kind":       resourcegroupv1alpha1.RGFileKind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]interface{}{
				common.InventoryLabel: live.InventoryID(),
			},
		}}), nil
}

func NewLiveApply(live *kuberikv1alpha1.Live, resMap resmap.ResMap) (*LiveApply, error) {
	resMap = resMap.DeepCopy()

	for _, r := range resMap.Resources() {
		if r.GetGvk().String() == resid.NewGvk(resourcegroupv1alpha1.RGFileGroup, resourcegroupv1alpha1.RGFileVersion, resourcegroupv1alpha1.RGFileKind).String() {
			return nil, fmt.Errorf("found ResourceGroup but one should be generated automatically")
		}
	}

	resourceGroup, err := generateResourceGroup(live)
	if err != nil {
		return nil, err
	}
	if err = resMap.Append(resourceGroup); err != nil {
		return nil, err
	}

	return &LiveApply{
		ResMap: resMap,
	}, nil
}
