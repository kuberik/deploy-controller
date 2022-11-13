package kustomize

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type LayeredFilesystem struct {
	Filesystems []filesys.FileSystem
}

var _ filesys.FileSystem = &LayeredFilesystem{}

// CleanedAbs implements filesys.FileSystem
func (l LayeredFilesystem) CleanedAbs(path string) (filesys.ConfirmedDir, string, error) {
	for _, fs := range l.Filesystems {
		if fs.Exists(path) {
			return fs.CleanedAbs(path)
		}
	}
	return "", "", fmt.Errorf("file not found: %s", path)
}

// Create implements filesys.FileSystem
func (LayeredFilesystem) Create(path string) (filesys.File, error) {
	panic("unimplemented")
}

// Exists implements filesys.FileSystem
func (LayeredFilesystem) Exists(path string) bool {
	panic("unimplemented")
}

// Glob implements filesys.FileSystem
func (LayeredFilesystem) Glob(pattern string) ([]string, error) {
	panic("unimplemented")
}

// IsDir implements filesys.FileSystem
func (LayeredFilesystem) IsDir(path string) bool {
	panic("unimplemented")
}

// Mkdir implements filesys.FileSystem
func (LayeredFilesystem) Mkdir(path string) error {
	panic("unimplemented")
}

// MkdirAll implements filesys.FileSystem
func (LayeredFilesystem) MkdirAll(path string) error {
	panic("unimplemented")
}

// Open implements filesys.FileSystem
func (LayeredFilesystem) Open(path string) (filesys.File, error) {
	panic("unimplemented")
}

// ReadDir implements filesys.FileSystem
func (LayeredFilesystem) ReadDir(path string) ([]string, error) {
	panic("unimplemented")
}

// ReadFile implements filesys.FileSystem
func (l LayeredFilesystem) ReadFile(path string) ([]byte, error) {
	for _, fs := range l.Filesystems {
		if fs.Exists(path) {
			return fs.ReadFile(path)
		}
	}
	return nil, fmt.Errorf("file not found: %s", path)
}

// RemoveAll implements filesys.FileSystem
func (LayeredFilesystem) RemoveAll(path string) error {
	panic("unimplemented")
}

// Walk implements filesys.FileSystem
func (LayeredFilesystem) Walk(path string, walkFn filepath.WalkFunc) error {
	panic("unimplemented")
}

// WriteFile implements filesys.FileSystem
func (LayeredFilesystem) WriteFile(path string, data []byte) error {
	panic("unimplemented")
}

type LocalConfigTransformOverlay struct {
	Base              Layer
	LocalConfigObject metav1.Object
	Transformers      string
}

func writeKustomization(fs filesys.FileSystem, dir string, kustomization types.Kustomization) error {
	kustomizationYaml, err := yaml.Marshal(kustomization)
	if err != nil {
		return err
	}
	return fs.WriteFile(filepath.Join(dir, konfig.DefaultKustomizationFileName()), kustomizationYaml)
}

func (o LocalConfigTransformOverlay) CreateLayeredFilesystemLayer() (*Layer, error) {
	tempFS := filesys.MakeFsInMemory()

	localConfigTransformLayerDir := "local-config-transform"
	localConfigTransformLayerAbsPath := filepath.Join(filepath.Dir(o.Base.Path), localConfigTransformLayerDir)
	transformers, err := filepath.Rel(localConfigTransformLayerAbsPath, o.Transformers)
	if err != nil {
		return nil, err
	}

	localConfigFile := "local-config.yaml"
	kustomization := types.Kustomization{
		Resources: []string{
			filepath.Join("..", filepath.Base(o.Base.Path)),
			localConfigFile,
		},
		Transformers: []string{transformers},
	}
	if err := writeKustomization(tempFS, localConfigTransformLayerAbsPath, kustomization); err != nil {
		return nil, err
	}

	localConfigObjectAnnotations := o.LocalConfigObject.GetAnnotations()
	if localConfigObjectAnnotations == nil {
		localConfigObjectAnnotations = make(map[string]string)
	}
	localConfigObjectAnnotations["config.kubernetes.io/local-config"] = "true"
	o.LocalConfigObject.SetAnnotations(localConfigObjectAnnotations)

	localConfigObjectYaml, err := json.Marshal(o.LocalConfigObject)
	if err != nil {
		return nil, err
	}
	if err := tempFS.WriteFile(filepath.Join(localConfigTransformLayerAbsPath, localConfigFile), localConfigObjectYaml); err != nil {
		return nil, err
	}

	return &Layer{
		FileSystem: &LayeredFilesystem{
			Filesystems: []filesys.FileSystem{
				tempFS,
				o.Base.FileSystem,
			},
		},
		Path: localConfigTransformLayerAbsPath,
	}, nil
}

type NameSuffixOverlay struct {
	Base       Layer
	NameSuffix string
}

func (o NameSuffixOverlay) CreateLayeredFilesystemLayer() (*Layer, error) {
	tempFS := filesys.MakeFsInMemory()

	nameSuffixLayerDir := "name-suffix"
	nameSuffixLayerAbsPath := filepath.Join(filepath.Dir(o.Base.Path), nameSuffixLayerDir)

	kustomization := types.Kustomization{
		Resources: []string{
			filepath.Join("..", filepath.Base(o.Base.Path)),
		},
		NameSuffix: fmt.Sprintf("-%s", o.NameSuffix),
	}
	if err := writeKustomization(tempFS, nameSuffixLayerAbsPath, kustomization); err != nil {
		return nil, err
	}
	return &Layer{
		FileSystem: LayeredFilesystem{
			Filesystems: []filesys.FileSystem{
				tempFS,
				o.Base.FileSystem,
			},
		},
		Path: nameSuffixLayerAbsPath,
	}, nil
}
