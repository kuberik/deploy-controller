package live

import "sigs.k8s.io/kustomize/api/provider"

var depProvider = provider.NewDefaultDepProvider()
var rf = depProvider.GetResourceFactory()
