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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	"github.com/kuberik/kuberik/pkg/repository"
)

// LiveDeploymentReconciler reconciles a LiveDeployment object
type LiveDeploymentReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	RepoDir string
}

//+kubebuilder:rbac:groups=kuberik.io,resources=livedeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kuberik.io,resources=livedeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kuberik.io,resources=livedeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LiveDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LiveDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	liveDeployment := &kuberikiov1alpha1.LiveDeployment{}
	err := r.Client.Get(ctx, req.NamespacedName, liveDeployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	auth, err := liveDeployment.Spec.Template.Spec.GetAuthMethod(ctx, r.Client, liveDeployment.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	repo, err := repository.InitGitRepository(path.Join(r.RepoDir, liveDeployment.Spec.Template.Spec.Repository.URL), liveDeployment.Spec.Template.Spec.Repository.URL, auth)
	if err != nil {
		return ctrl.Result{}, err
	}

	commitSHA, err := repo.FetchBranch(liveDeployment.Spec.Branch)
	if err != nil {
		return ctrl.Result{}, err
	}

	generatedLive := liveDeployment.CreateLiveForCommit(*commitSHA)
	err = r.Client.Create(ctx, generatedLive)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existingLive := &kuberikiov1alpha1.Live{}
			err = r.Client.Get(ctx, req.NamespacedName, existingLive)
			if err != nil {
				return ctrl.Result{}, err
			}

			generatedLive.Spec.DeepCopyInto(&existingLive.Spec)
			err = r.Client.Update(ctx, existingLive)
			if err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(liveDeployment.Spec.PollIntervalSeconds+1) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LiveDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kuberikiov1alpha1.LiveDeployment{}).
		Complete(r)
}
