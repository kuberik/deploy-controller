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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LiveSpec defines the desired state of Live
type LiveSpec struct {
	// Relative path of the kustomize layer within the specified git repository which will
	// be applied to the cluster.
	Path string `json:"path,omitempty"`

	// Commit of the git repository that will be checked out to deploy kustomize layer from.
	Commit string `json:"commit,omitempty"`

	// Git repository containing the kustomize layer that needs to be deployed
	Repository `json:"repository,omitempty"`

	// Interruptible defines if the Live can be updated while it is already actively reconciling
	Interruptible bool `json:"interruptible,omitempty"`

	// Transformers define kustomize transformer layer which will be used to transform the specified kustomize layer.
	// The path specified needs to be relative path in the git repository.
	// Live object will be included in the Kustomize layers with annotation <code>config.kubernetes.io/local-config=true</code>
	// so that the transformers (most notably builtin <code>ReplacementTransformer</code>) can use the information from the Live
	// objects (such as git commit hash).
	Transformers string `json:"transformers,omitempty"`

	// Name of the ServiceAccount to use for deploying the resources.
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// LiveStatus defines the observed state of Live
type LiveStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions is a list of conditions on the Live resource
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type LivePhaseName string

type LivePhase struct {
	Name         LivePhaseName
	ExtraMessage string
}

func (lp *LivePhase) formatMessage() string {
	var messageBase string
	switch lp.Name {
	case LivePhaseApplying:
		messageBase = "Applying the resources"
	case LivePhaseSucceeded:
		messageBase = "Apply complete"
	case LivePhaseFailed:
		messageBase = "Failed to apply the resources"
	}
	if lp.ExtraMessage != "" {
		return fmt.Sprintf("%s: %s", messageBase, lp.ExtraMessage)
	}
	return messageBase
}

const (
	LivePhaseApplying  LivePhaseName = "Applying"
	LivePhaseSucceeded LivePhaseName = "Succeeded"
	LivePhaseFailed    LivePhaseName = "Failed"
)

//+k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=li
//+kubebuilder:printcolumn:name="Commit",type="string",JSONPath=".spec.commit",description=""
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].reason",description=""
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""

// Live is deploying a single Kustomize layer from a commit in a git repository.
// ::: warning
// It is recommended that users create Lives only through a Controller, and not directly.
// See Controllers: [LiveDeployment](#kuberik-io-v1alpha1-LiveDeployment),
// [LiveDeploymentGroup](#kuberik-io-v1alpha1-LiveDeploymentGroup).
// :::
type Live struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the Live.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec LiveSpec `json:"spec,omitempty"`
	// Most recently observed status of the Live. This data may not be up to date. Populated by the system. Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Status LiveStatus `json:"status,omitempty"`
}

// LiveConditionType is the type of the condition
type LiveConditionType string

const (
	// LiveConditionReady is set when the Live is reconciled with the specified commit
	LiveConditionReady LiveConditionType = "Ready"
)

func (l *Live) GetReadyCondition() *metav1.Condition {
	return meta.FindStatusCondition(l.Status.Conditions, string(LiveConditionReady))
}

func (l *Live) Reconciled() bool {
	condition := l.GetReadyCondition()
	if condition == nil {
		return false
	}

	return condition.Status == metav1.ConditionTrue && condition.ObservedGeneration == l.Generation
}

func (l *Live) IsApplying() bool {
	condition := l.GetReadyCondition()
	if condition == nil {
		return false
	}

	return condition.Reason == string(LivePhaseApplying)
}

func (l *Live) InventoryID() string {
	return string(l.UID)
}

func (l *Live) SetPhase(phase LivePhase) {
	var status metav1.ConditionStatus
	switch phase.Name {
	case LivePhaseApplying:
		l.Status.Conditions = []metav1.Condition{}
		status = metav1.ConditionFalse
	case LivePhaseSucceeded:
		status = metav1.ConditionTrue
	case LivePhaseFailed:
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&l.Status.Conditions, metav1.Condition{
		Type:               string(LiveConditionReady),
		Status:             status,
		Reason:             string(phase.Name),
		Message:            phase.formatMessage(),
		ObservedGeneration: l.Generation,
	})
}

func (l *Live) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: l.Name, Namespace: l.Namespace}
}

func (l *Live) GetServiceAccountName() string {
	if l.Spec.ServiceAccountName == "" {
		return "default"
	}
	return l.Spec.ServiceAccountName
}

//+kubebuilder:object:root=true

// LiveList contains a list of Live
type LiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Live `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Live{}, &LiveList{})
}
