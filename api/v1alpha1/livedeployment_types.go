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
	"github.com/go-git/go-git/v5/plumbing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LiveDeploymentSpec defines the desired state of LiveDeployment
type LiveDeploymentSpec struct {
	// Branch of the git repository specified in the Live template that will be continuously deployed.
	Branch string `json:"branch,omitempty"`

	// Template of the created Live resource that will be used to deploy latest commit from the specified branch.
	Template *LiveTemplate `json:"template,omitempty"`

	// The duration of seconds between each fetching of the git repository.
	PollIntervalSeconds int32 `json:"pollIntervalSeconds,omitempty"`
}

// LiveTemplate describes a Live that will be created
type LiveTemplate struct {
	// Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the desired behavior of the Live.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec LiveSpec `json:"spec,omitempty"`
}

// LiveDeploymentStatus defines the observed state of LiveDeployment
type LiveDeploymentStatus struct {
}

//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=ld
//+kubebuilder:printcolumn:name="Branch",type="string",JSONPath=".spec.branch",description=""
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""

// LiveDeployment is continously deploying a single Kustomize layer from a branch
// every time the tip of the branch updates.
type LiveDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the LiveDeployment.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec LiveDeploymentSpec `json:"spec,omitempty"`
	// Most recently observed status of the LiveDeployment. This data may not be up to date. Populated by the system. Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Status LiveDeploymentStatus `json:"status,omitempty"`
}

func (l *LiveDeployment) CreateLiveForCommit(commitSHA plumbing.Hash) *Live {
	spec := l.Spec.Template.Spec.DeepCopy()
	spec.Commit = commitSHA.String()
	spec.Repository = *l.Spec.Template.Spec.Repository.DeepCopy()
	metadata := l.Spec.Template.ObjectMeta.DeepCopy()
	metadata.Name = l.Name
	metadata.Namespace = l.Namespace
	metadata.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(l, GroupVersion.WithKind(LiveDeploymentKind)),
	}

	return &Live{
		ObjectMeta: *metadata,
		Spec:       *spec,
	}
}

//+kubebuilder:object:root=true

// LiveDeploymentList contains a list of LiveDeployment
type LiveDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LiveDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LiveDeployment{}, &LiveDeploymentList{})
}
