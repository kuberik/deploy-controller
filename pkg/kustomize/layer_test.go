package kustomize

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func TestKustomizeBuild(t *testing.T) {
	testCases := []struct {
		buildPath string
		want      string
		wantErr   bool
		files     fstest.MapFS
	}{
		{
			buildPath: "empty",
			want:      "",
			files: fstest.MapFS{
				"empty/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
`)},
			},
		}, {
			buildPath: "base",
			want: strings.TrimSpace(`
apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  name: config
			`),
			files: fstest.MapFS{
				"base/kustomization.yml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: config
  options:
    disableNameSuffixHash: true
  literals:
  - foo=bar
`)},
			},
		}, {
			buildPath: "overlay",
			want: strings.TrimSpace(`
apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  name: config
			`),
			files: fstest.MapFS{
				"base/kustomization.yml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: config
  options:
    disableNameSuffixHash: true
  literals:
  - foo=bar
`)},
				"overlay/Kustomization": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../base
`)},
			},
		}, {
			buildPath: "",
			want: strings.TrimSpace(`
apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  name: config
			`),
			files: fstest.MapFS{
				"kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ./base
`)},
				"base/kustomization.yml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
configMapGenerator:
- name: config
  options:
    disableNameSuffixHash: true
  literals:
  - foo=bar`)},
			},
		}, {
			buildPath: "invalid",
			wantErr:   true,
			files: fstest.MapFS{
				"invalid/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../foo.yaml
	`)},
			},
		}, {
			buildPath: "non-existant",
			wantErr:   true,
		}, {
			buildPath: "no-kustomization",
			wantErr:   true,
			files: fstest.MapFS{
				"no-kustomization/resource.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
	`)},
			},
		}, {
			buildPath: "base",
			want: strings.TrimSpace(`
apiVersion: v1
kind: Namespace
metadata:
  name: foo
			`),
			files: fstest.MapFS{
				"base/kustomization.yaml": {Data: []byte(`
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: foo
resources:
- namespace.yaml
`)},
				"base/namespace.yaml": {Data: []byte(`
apiVersion: v1
kind: Namespace
metadata:
  name: foo
`)},
			},
		},
	}

	for _, tc := range testCases {
		layer := Layer{
			FileSystem: MapFSToKustomizeMemoryFilesystem(t, tc.files),
			Path:       tc.buildPath,
		}

		result, err := layer.Build()
		if tc.wantErr {
			if err == nil {
				t.Errorf("KustomizeBuild(%q) succeeded, expected error", tc.buildPath)
			}
			if result != nil {
				t.Errorf("KustomizeBuild(%q) returned non-nil result, expected nil", tc.buildPath)
			}
			continue
		} else if err != nil {
			t.Errorf("KustomizeBuild(%q) failed: %v", tc.buildPath, err)
		}

		got, err := result.ResMap.AsYaml()
		if err != nil {
			t.Errorf("KustomizeBuild(%q).AsYaml() failed: %v", tc.buildPath, err)
		}

		if diff := cmp.Diff(tc.want, strings.TrimSpace(string(got))); diff != "" {
			t.Errorf("KustomizeBuild(%q) mismatch (-want +got):\n%s", tc.buildPath, diff)
		}
	}
}
