package kustomize

import (
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/kustomize/v4/commands/build"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type Layer struct {
	Path string
	filesys.FileSystem
}

func (l *Layer) Build() (*KustomizeBuild, error) {
	k := krusty.MakeKustomizer(
		build.HonorKustomizeFlags(krusty.MakeDefaultOptions()),
	)

	path := l.Path
	if l.Path == "" {
		path = "."
	}

	m, err := k.Run(l.FileSystem, path)
	if err != nil {
		return nil, err
	}

	return &KustomizeBuild{
		ResMap: m,
	}, nil
}

type KustomizeBuild struct {
	ResMap resmap.ResMap
}
