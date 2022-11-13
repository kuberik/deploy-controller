/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LiveDeploymentGroupSpec defines the desired state of LiveDeploymentGroup
type LiveDeploymentGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Regex pattern used to match branches that will be deployed
	BranchMatch string `json:"branchMatch,omitempty"`

	// Template of the created Live resources that will be used to deploy latest commit from each matching branch.
	Template *LiveTemplate `json:"template,omitempty"`

	// The duration in seconds between each fetching of the git repository.
	PollIntervalSeconds int32 `json:"pollIntervalSeconds,omitempty"`
}

// LiveDeploymentGroupStatus defines the observed state of LiveDeploymentGroup
type LiveDeploymentGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

const (
	LiveDeploymentGroupLabel = "kuberik.io/live-deployment-group"
)

func (ldg *LiveDeploymentGroup) LiveDeploymentForBranch(branch string) *LiveDeployment {
	return &LiveDeployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", ldg.Name),
			Namespace:    ldg.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ldg, GroupVersion.WithKind(LiveDeploymentGroupKind)),
			},
			Labels: ldg.liveDeploymentLabels(),
		},
		Spec: LiveDeploymentSpec{
			Branch:              branch,
			Template:            ldg.Spec.Template.DeepCopy(),
			PollIntervalSeconds: ldg.Spec.PollIntervalSeconds,
		},
	}
}

func (ldg *LiveDeploymentGroup) liveDeploymentLabels() labels.Set {
	return map[string]string{
		LiveDeploymentGroupLabel: ldg.Name,
	}
}

func (ldg *LiveDeploymentGroup) LiveDeploymentSelector() labels.Selector {
	return ldg.liveDeploymentLabels().AsSelector()
}

//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// LiveDeploymentGroup is deploying multiple Kustomize layers, each from the same path but
// from a different branch of a git repository.
// ::: details Example
// ```yaml
// <!-- @include: ../../../manifests/ci/all-branches/ci.yaml -->
// ```
// :::
type LiveDeploymentGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the LiveDeploymentGroup.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec LiveDeploymentGroupSpec `json:"spec,omitempty"`
	// Most recently observed status of the LiveDeploymentGroup. This data may not be up to date. Populated by the system. Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Status LiveDeploymentGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// LiveDeploymentGroupList contains a list of LiveDeploymentGroup
type LiveDeploymentGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LiveDeploymentGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LiveDeploymentGroup{}, &LiveDeploymentGroupList{})
}
