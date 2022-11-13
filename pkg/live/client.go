package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	kptfilev1 "github.com/GoogleContainerTools/kpt/pkg/api/kptfile/v1"
	resourcegroupv1alpha1 "github.com/GoogleContainerTools/kpt/pkg/api/resourcegroup/v1alpha1"
	"github.com/GoogleContainerTools/kpt/pkg/live"
	"github.com/GoogleContainerTools/kpt/pkg/status"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"sigs.k8s.io/cli-utils/pkg/apply"
	"sigs.k8s.io/cli-utils/pkg/common"
	"sigs.k8s.io/cli-utils/pkg/inventory"
	"sigs.k8s.io/cli-utils/pkg/object"

	// statscommon "sigs.k8s.io/cli-utils/pkg/print/common"

	"sigs.k8s.io/cli-utils/pkg/printers"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var _ genericclioptions.RESTClientGetter = &RestConfigClientGetter{}

type RestConfigClientGetter struct {
	config rest.Config
}

func NewRestConfigClientGetter(config rest.Config) *RestConfigClientGetter {
	return &RestConfigClientGetter{
		config: config,
	}
}

// ToDiscoveryClient implements genericclioptions.RESTClientGetter
func (c *RestConfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	client, err := kubernetes.NewForConfig(&c.config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client.Discovery()), nil
}

// ToRESTConfig implements genericclioptions.RESTClientGetter
func (c *RestConfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return &c.config, nil
}

// ToRESTMapper implements genericclioptions.RESTClientGetter
func (c *RestConfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discovery, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(discovery), nil
}

// ToRawKubeConfigLoader implements genericclioptions.RESTClientGetter
func (c *RestConfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	var config *api.Config
	if c.config.BearerToken != "" {
		config = kubeconfig.CreateWithToken(
			c.config.Host,
			c.config.ServerName,
			c.config.Username,
			c.config.CertData,
			c.config.BearerToken,
		)
	} else if len(c.config.CAData) > 0 && len(c.config.KeyData) > 0 && len(c.config.CertData) > 0 {
		config = kubeconfig.CreateWithCerts(
			c.config.Host,
			c.config.ServerName,
			c.config.Username,
			c.config.CAData,
			c.config.KeyData,
			c.config.CertData,
		)
	}
	raw, err := runtime.Encode(clientcmdlatest.Codec, config)
	if err != nil {
		panic(err)
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(raw)
	if err != nil {
		panic(err)
	}
	return clientConfig
}

type KptClient struct {
	ctx                       context.Context
	resourceGroupClientGetter genericclioptions.RESTClientGetter
	resourceClientGetter      genericclioptions.RESTClientGetter
}

func NewKptClient(ctx context.Context, config rest.Config) (*KptClient, error) {
	return &KptClient{
		resourceGroupClientGetter: NewRestConfigClientGetter(config),
		resourceClientGetter:      NewRestConfigClientGetter(config),
		ctx:                       ctx,
	}, nil
}

func (c *KptClient) ImpersonateForResources(impersonateServiceAccount types.NamespacedName) error {
	config, err := c.resourceGroupClientGetter.ToRESTConfig()
	if err != nil {
		return err
	}

	impersonateConfig := *rest.CopyConfig(config)
	impersonateConfig.Impersonate = rest.ImpersonationConfig{
		UserName: serviceaccount.MakeUsername(impersonateServiceAccount.Namespace, impersonateServiceAccount.Name),
	}

	c.resourceClientGetter = NewRestConfigClientGetter(impersonateConfig)
	return nil
}

type kptApplyObjects struct {
	objects       object.UnstructuredSet
	resourceGroup *resource.Resource
}

func newKptApplyObjects(resMap resmap.ResMap) (*kptApplyObjects, error) {
	var applySet []*resource.Resource
	var resourceGroup *resource.Resource
	for _, r := range resMap.Resources() {
		if r.GetKind() == resourcegroupv1alpha1.RGFileKind && r.GetApiVersion() == resourcegroupv1alpha1.RGFileAPIVersion {
			if resourceGroup != nil {
				return nil, fmt.Errorf("multiple resource groups found")
			}
			resourceGroup = r
			continue
		}

		applySet = append(applySet, r)
	}
	if resourceGroup == nil {
		return nil, fmt.Errorf("no resource group found")
	}

	var objects object.UnstructuredSet
	for _, r := range applySet {
		obj, err := kyamlNodeToUnstructured(&r.RNode)
		if err != nil {
			return nil, err
		}
		objects = append(objects, obj)
	}

	return &kptApplyObjects{
		objects:       objects,
		resourceGroup: resourceGroup,
	}, nil
}

func kyamlNodeToUnstructured(n *yaml.RNode) (*unstructured.Unstructured, error) {
	b, err := n.MarshalJSON()
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{
		Object: m,
	}, nil
}

type ApplyOptions struct {
	Timeout      time.Duration
	PruneTimeout time.Duration
}

func (c *KptClient) Apply(resMap resmap.ResMap, options ApplyOptions) error {
	invClientFactory := util.NewFactory(c.resourceGroupClientGetter)
	invClient, err := inventory.NewClient(invClientFactory, live.WrapInventoryObj, live.InvToUnstructuredFunc, inventory.StatusPolicyAll, live.ResourceGroupGVK)
	if err != nil {
		return err
	}

	applyObjects, err := newKptApplyObjects(resMap)
	if err != nil {
		return err
	}

	invInfo, err := live.ToInventoryInfo(kptfilev1.Inventory{
		Name:        applyObjects.resourceGroup.GetName(),
		Namespace:   applyObjects.resourceGroup.GetNamespace(),
		InventoryID: applyObjects.resourceGroup.GetLabels()[common.InventoryLabel],
	})
	if err != nil {
		return err
	}

	factory := util.NewFactory(c.resourceClientGetter)
	applier, err := apply.NewApplierBuilder().
		WithFactory(factory).
		WithInventoryClient(invClient).
		Build()
	if err != nil {
		return err
	}

	dryRunStrategy := common.DryRunNone
	ch, err := applier.Run(c.ctx, invInfo, applyObjects.objects, apply.ApplierOptions{
		// TODO: Use server-side
		ServerSideOptions:      common.ServerSideOptions{},
		ReconcileTimeout:       options.Timeout,
		EmitStatusEvents:       true, // We are always waiting for reconcile.
		DryRunStrategy:         dryRunStrategy,
		PrunePropagationPolicy: metav1.DeletePropagationBackground,
		PruneTimeout:           options.PruneTimeout,
		InventoryPolicy:        inventory.PolicyAdoptIfNoInventory,
	}), nil
	if err != nil {
		return err
	}
	printer := printers.GetPrinter(printers.DefaultPrinter(), genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	return printer.Print(ch, dryRunStrategy, true)
}

func (a *KptClient) installResourceGroup(f util.Factory) error {
	return (&live.ResourceGroupInstaller{
		Factory: f,
	}).InstallRG(a.ctx)
}

func (a *KptClient) InstallResourceGroup() error {
	f := util.NewFactory(a.resourceGroupClientGetter)
	// Install the ResourceGroup CRD if it is not already installed
	// or if the ResourceGroup CRD doesn't match the CRD in the
	// kpt binary.
	if !live.ResourceGroupCRDApplied(f) {
		if err := a.installResourceGroup(f); err != nil {
			return err
		}
	} else if !live.ResourceGroupCRDMatched(f) {
		if err := a.installResourceGroup(f); err != nil {
			return err
		}
	}
	return nil
}

func (c *KptClient) Destroy(object types.NamespacedName, id string) error {
	invClientFactory := util.NewFactory(c.resourceGroupClientGetter)
	invClient, err := inventory.NewClient(invClientFactory, live.WrapInventoryObj, live.InvToUnstructuredFunc, inventory.StatusPolicyAll, live.ResourceGroupGVK)
	if err != nil {
		return err
	}

	factory := util.NewFactory(c.resourceClientGetter)
	statusWatcher, err := status.NewStatusWatcher(factory)
	if err != nil {
		return err
	}

	destroyer, err := apply.NewDestroyerBuilder().
		WithFactory(factory).
		WithInventoryClient(invClient).
		WithStatusWatcher(statusWatcher).
		Build()
	if err != nil {
		return err
	}

	dryRunStrategy := common.DryRunNone
	options := apply.DestroyerOptions{
		InventoryPolicy:  inventory.PolicyAdoptIfNoInventory,
		DryRunStrategy:   dryRunStrategy,
		EmitStatusEvents: true,
		// DeletePropagationPolicy: metav1.DeletePropagationForeground,
	}
	inv, err := live.ToInventoryInfo(kptfilev1.Inventory{
		Name:        object.Name,
		Namespace:   object.Namespace,
		InventoryID: id,
	})
	if err != nil {
		return err
	}
	ch := destroyer.Run(c.ctx, inv, options)

	printer := printers.GetPrinter(printers.DefaultPrinter(), genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	return printer.Print(ch, dryRunStrategy, true)
}
