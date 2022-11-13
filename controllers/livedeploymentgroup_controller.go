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

package controllers

import (
	"context"
	"path"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	"github.com/kuberik/kuberik/pkg/repository"
)

// LiveDeploymentGroupReconciler reconciles a LiveDeploymentGroup object
type LiveDeploymentGroupReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	RepoDir string
}

//+kubebuilder:rbac:groups=kuberik.io,resources=livedeploymentgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kuberik.io,resources=livedeploymentgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kuberik.io,resources=livedeploymentgroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LiveDeploymentGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LiveDeploymentGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	liveDeploymentGroup := &kuberikiov1alpha1.LiveDeploymentGroup{}
	err := r.Client.Get(ctx, req.NamespacedName, liveDeploymentGroup)
	if err != nil {
		return ctrl.Result{}, err
	}

	auth, err := liveDeploymentGroup.Spec.Template.Spec.GetAuthMethod(ctx, r.Client, liveDeploymentGroup.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	repo, err := repository.InitGitRepository(path.Join(r.RepoDir, liveDeploymentGroup.Spec.Template.Spec.Repository.URL), liveDeploymentGroup.Spec.Template.Spec.Repository.URL, auth)
	if err != nil {
		return ctrl.Result{}, err
	}

	branches, err := repo.ListBranches(liveDeploymentGroup.Spec.BranchMatch)
	if err != nil {
		return ctrl.Result{}, err
	}

	createdLiveDeployments := &kuberikiov1alpha1.LiveDeploymentList{}
	if err := r.Client.List(ctx, createdLiveDeployments, &client.ListOptions{
		LabelSelector: liveDeploymentGroup.LiveDeploymentSelector(),
	}); err != nil {
		return ctrl.Result{}, nil
	}

branches:
	for _, b := range branches {
		for _, ld := range createdLiveDeployments.Items {
			if ld.Spec.Branch == b {
				continue branches
			}
		}
		if err := r.Client.Create(ctx, liveDeploymentGroup.LiveDeploymentForBranch(b)); err != nil {
			return ctrl.Result{}, err
		}
	}

liveDeployments:
	for _, ld := range createdLiveDeployments.Items {
		for _, b := range branches {
			if ld.Spec.Branch == b {
				continue liveDeployments
			}
		}
		if err := r.Client.Delete(ctx, &ld); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(liveDeploymentGroup.Spec.PollIntervalSeconds+1) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LiveDeploymentGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuberikiov1alpha1.LiveDeploymentGroup{}).
		Complete(r)
}
