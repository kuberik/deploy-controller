package kustomize

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var depProvider = provider.NewDefaultDepProvider()
var rf = depProvider.GetResourceFactory()

func MapFSToKustomizeMemoryFilesystem(t *testing.T, files fstest.MapFS) filesys.FileSystem {
	fs := filesys.MakeFsInMemory()
	for path, content := range files {
		err := fs.MkdirAll(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		err = fs.WriteFile(path, content.Data)
		if err != nil {
			t.Fatal(err)
		}
	}
	return fs
}
