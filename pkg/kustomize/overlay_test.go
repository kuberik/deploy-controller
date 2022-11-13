package kustomize

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/kuberik/kuberik/api/v1alpha1"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestOverlayFilesystemBuild(t *testing.T) {
	layeredFS := LayeredFilesystem{
		Filesystems: []filesys.FileSystem{
			MapFSToKustomizeMemoryFilesystem(t, fstest.MapFS{
				"kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- pod.yaml
`)},
			}),
			MapFSToKustomizeMemoryFilesystem(t, fstest.MapFS{
				"pod.yaml": {Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - image: my-app:latest
    name: my-app
`)},
			}),
		},
	}

	want := strings.TrimSpace(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - image: my-app:latest
    name: my-app
`)

	layer := Layer{
		FileSystem: layeredFS,
	}

	build, err := layer.Build()
	assert.NilError(t, err)

	got, err := build.ResMap.AsYaml()
	assert.NilError(t, err)

	if diff := cmp.Diff(want, strings.TrimSpace(string(got))); diff != "" {
		t.Errorf("GitOverlay.Build mismatch (-want +got):\n%s", diff)
	}
}

func TestLocalConfigTransformOverlay(t *testing.T) {
	testCases := []struct {
		overlay LocalConfigTransformOverlay
		want    string
		wantErr bool
	}{
		{
			overlay: LocalConfigTransformOverlay{
				Base: Layer{
					Path: "pod-image-tag",
					FileSystem: MapFSToKustomizeMemoryFilesystem(t, fstest.MapFS{
						"pod-image-tag/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- pod.yaml
`)},
						"pod-image-tag/pod.yaml": {Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - name: my-app
    image: my-app:latest
`)},
						"pod-image-tag/transformers/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- replace.yaml
`)},
						"pod-image-tag/transformers/replace.yaml": {Data: []byte(`
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: notImportantHere
replacements:
- source:
    kind: Live
    fieldPath: spec.commit
  targets:
  - select:
      kind: Pod
    fieldPaths:
    - spec.containers.[name=my-app].image
    options:
      delimiter: ":"
      index: 1
`)},
					}),
				},
				LocalConfigObject: &v1alpha1.Live{
					TypeMeta: metav1.TypeMeta{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       v1alpha1.LiveKind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "reconcile",
						Namespace: "default",
					},
					Spec: v1alpha1.LiveSpec{
						Repository: v1alpha1.Repository{
							URL: "https://github.com/kubernetes-sigs/kustomize.git",
						},
						Commit: "891971f25da7a17e05bb96b545e759e63d2ef5b7",
					},
				},
				Transformers: "pod-image-tag/transformers",
			},
			want: strings.TrimSpace(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - image: my-app:891971f25da7a17e05bb96b545e759e63d2ef5b7
    name: my-app
			`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.overlay.Base.Path, func(t *testing.T) {
			layer, err := tc.overlay.CreateLayeredFilesystemLayer()
			assert.NilError(t, err)

			result, err := layer.Build()
			if tc.wantErr {
				if err == nil {
					t.Errorf("LocalConfigTransformOverlay build succeeded, expected error")
				}
				if result != nil {
					t.Errorf("LocalConfigTransformOverlay build returned non-nil result, expected nil")
				}
				return
			} else if err != nil {
				t.Errorf("LocalConfigTransformOverlay build failed: %v", err)
			}

			got, err := result.ResMap.AsYaml()
			if err != nil {
				t.Errorf("LocalConfigTransformOverlay build.AsYaml() failed: %v", err)
			}

			if diff := cmp.Diff(tc.want, strings.TrimSpace(string(got))); diff != "" {
				t.Errorf("LocalConfigTransformOverlay build mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNameSuffixOverlay(t *testing.T) {
	testCases := []struct {
		overlay NameSuffixOverlay
		want    string
		wantErr bool
	}{
		{
			overlay: NameSuffixOverlay{
				Base: Layer{
					Path: "pod-image-tag",
					FileSystem: MapFSToKustomizeMemoryFilesystem(t, fstest.MapFS{
						"pod-image-tag/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- pod.yaml
`)},
						"pod-image-tag/pod.yaml": {Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - name: my-app
    image: my-app:latest
`)},
					}),
				},
				NameSuffix: "test",
			},
			want: strings.TrimSpace(`
apiVersion: v1
kind: Pod
metadata:
  name: my-app-test
spec:
  containers:
  - image: my-app:latest
    name: my-app
`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.overlay.Base.Path, func(t *testing.T) {
			layer, err := tc.overlay.CreateLayeredFilesystemLayer()
			assert.NilError(t, err)

			result, err := layer.Build()
			if tc.wantErr {
				if err == nil {
					t.Errorf("NameSuffixOverlay build succeeded, expected error")
				}
				if result != nil {
					t.Errorf("NameSuffixOverlay build returned non-nil result, expected nil")
				}
				return
			} else if err != nil {
				t.Errorf("NameSuffixOverlay build failed: %v", err)
			}

			got, err := result.ResMap.AsYaml()
			if err != nil {
				t.Errorf("NameSuffixOverlay build.AsYaml() failed: %v", err)
			}

			if diff := cmp.Diff(tc.want, strings.TrimSpace(string(got))); diff != "" {
				t.Errorf("NameSuffixOverlay build mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
