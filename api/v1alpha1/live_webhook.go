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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var livelog = logf.Log.WithName("live-resource")

func (r *Live) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-kuberik-io-v1alpha1-live,mutating=true,failurePolicy=fail,sideEffects=None,groups=kuberik.io,resources=lives,verbs=create;update,versions=v1alpha1,name=mlive.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Live{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Live) Default() {
	livelog.Info("default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-kuberik-io-v1alpha1-live,mutating=false,failurePolicy=fail,sideEffects=None,groups=kuberik.io,resources=lives,verbs=create;update,versions=v1alpha1,name=vlive.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Live{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Live) ValidateCreate() error {
	livelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Live) ValidateUpdate(old runtime.Object) error {
	livelog.Info("validate update", "name", r.Name)

	oldLive := old.(*Live)

	if !oldLive.CanInterrupt() && !r.CanInterrupt() {
		return fmt.Errorf("previous apply is not complete")
	}

	if oldLive.Spec.ServiceAccountName != r.Spec.ServiceAccountName {
		return fmt.Errorf("not allowed to change serviceAccountName")
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Live) ValidateDelete() error {
	panic("unimplmented")
}

func (r *Live) CanInterrupt() bool {
	return r.Spec.Interruptible || !r.IsApplying()
}
