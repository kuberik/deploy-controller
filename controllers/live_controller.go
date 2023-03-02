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
	"fmt"
	"path"

	"github.com/go-git/go-git/v5/plumbing"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	// "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	"github.com/kuberik/kuberik/pkg/kustomize"
	livepkg "github.com/kuberik/kuberik/pkg/live"
	"github.com/kuberik/kuberik/pkg/repository"
)

const LiveDestroyFinalizer = "kuberik.io/live-destroy"

// LiveReconciler reconciles a Live object
type LiveReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Config          *rest.Config
	RepoDir         string
	ApplyResults    map[types.NamespacedName]<-chan error
	DeleteResults   map[types.NamespacedName]<-chan error
	KptClientEvents chan event.GenericEvent
}

//+kubebuilder:rbac:groups=kuberik.io,resources=lives,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kuberik.io,resources=lives/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kuberik.io,resources=lives/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Live object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Live")

	live := &kuberikiov1alpha1.Live{}
	err := r.Client.Get(ctx, req.NamespacedName, live)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed fetching live resource: %v", err)
	}
	if err := r.SetFinalizers(ctx, live); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set finalizers: %v", err)
	}

	if !live.Reconciled() {
		return r.ReconcileApply(ctx, live)
	}

	if live.DeletionTimestamp != nil {
		return r.ReconcileDelete(ctx, live)
	}

	return ctrl.Result{}, nil
}

func (r *LiveReconciler) SetFinalizers(ctx context.Context, live *kuberikiov1alpha1.Live) error {
	if live.DeletionTimestamp != nil {
		return nil
	}

	for _, f := range live.Finalizers {
		if f == LiveDestroyFinalizer {
			return nil
		}
	}
	live.Finalizers = append(live.Finalizers, LiveDestroyFinalizer)
	return r.Update(ctx, live)
}

func (r *LiveReconciler) ReconcileChanResult(
	ctx context.Context,
	live *kuberikiov1alpha1.Live,
	results map[types.NamespacedName]<-chan error,
	reconcile func(error) error,
) error {
	namespacedName := live.NamespacedName()
	select {
	case result := <-results[namespacedName]:
		if err := reconcile(result); err != nil {
			ch := make(chan error, 1)
			ch <- result
			close(ch)
			results[namespacedName] = ch
			return err
		}
		delete(results, namespacedName)
	default:
	}
	return nil
}

func (r *LiveReconciler) ReconcileApply(ctx context.Context, live *kuberikiov1alpha1.Live) (ctrl.Result, error) {
	if _, ok := r.ApplyResults[live.NamespacedName()]; ok {
		return ctrl.Result{}, r.ReconcileChanResult(ctx, live, r.ApplyResults, func(err error) error {
			if err != nil {
				live.SetPhase(kuberikiov1alpha1.LivePhase{
					Name:         kuberikiov1alpha1.LivePhaseFailed,
					ExtraMessage: err.Error(),
				})
			} else {
				live.SetPhase(kuberikiov1alpha1.LivePhase{Name: kuberikiov1alpha1.LivePhaseSucceeded})
			}
			return r.Client.Status().Update(ctx, live)
		})
	}

	auth, err := live.Spec.GetAuthMethod(ctx, r.Client, live.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get auth method: %v", err)
	}
	repo, err := repository.InitGitRepository(path.Join(r.RepoDir, live.Namespace, live.Name), live.Spec.Repository.URL, auth)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init git repository: %v", err)
	}

	err = repo.FetchCommit(live.Spec.Commit)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch commit: %v", err)
	}

	commitDir, err := repo.CreateCommitDir(plumbing.NewHash(live.Spec.Commit))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to commit dir: %v", err)
	}

	baseLayer := kustomize.Layer{
		FileSystem: filesys.MakeFsOnDisk(),
		Path:       path.Join(commitDir, live.Spec.Path),
	}
	buildLayer := baseLayer
	if live.Spec.Transformers != "" {
		transformOverlay := kustomize.LocalConfigTransformOverlay{
			Base:              baseLayer,
			LocalConfigObject: live.DeepCopy(),
			Transformers:      path.Join(commitDir, live.Spec.Transformers),
		}
		transformOverlayLayer, err := transformOverlay.CreateLayeredFilesystemLayer()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create transform overlay: %v", err)
		}
		buildLayer = *transformOverlayLayer
	}

	build, err := buildLayer.Build()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("kustomize build failed: %v", err)
	}

	if err := r.InstallResourceGroup(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to install resource group: %v", err)
	}

	apply, err := livepkg.NewLiveApply(live, build.ResMap)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply resources: %v", err)
	}

	live.SetPhase(kuberikiov1alpha1.LivePhase{Name: kuberikiov1alpha1.LivePhaseApplying})
	if err := r.Client.Status().Update(ctx, live); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set state to applying: %v", err)
	}

	result := make(chan error, 1)
	r.ApplyResults[live.NamespacedName()] = result
	kptClient, err := r.GetKptClient(ctx, *live)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create apply client: %v", err)
	}
	go func() {
		// TODO: Set options from LiveSpec
		result <- kptClient.Apply(apply.ResMap, livepkg.ApplyOptions{})
		close(result)
		r.KptClientEvents <- event.GenericEvent{Object: live}
	}()

	return ctrl.Result{}, nil
}

func (r *LiveReconciler) ReconcileDelete(ctx context.Context, live *kuberikiov1alpha1.Live) (ctrl.Result, error) {
	namespacedName := types.NamespacedName{Name: live.Name, Namespace: live.Namespace}
	if _, ok := r.DeleteResults[namespacedName]; !ok {
		kptClient, err := r.GetKptClient(ctx, *live)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create kpt client: %v", err)
		}
		result := make(chan error, 1)
		r.DeleteResults[namespacedName] = result
		go func() {
			result <- kptClient.Destroy(namespacedName, live.InventoryID())
			close(result)
			r.KptClientEvents <- event.GenericEvent{Object: live}
		}()
	}

	return ctrl.Result{}, r.ReconcileChanResult(ctx, live, r.DeleteResults, func(err error) error {
		finalizers := []string{}
		for _, f := range live.Finalizers {
			if f != LiveDestroyFinalizer {
				finalizers = append(finalizers, f)
			}
		}
		live.Finalizers = finalizers
		return r.Client.Update(ctx, live)
	})
}

func (r *LiveReconciler) InstallResourceGroup(ctx context.Context) error {
	kptClient, err := livepkg.NewKptClient(ctx, *r.Config)
	if err != nil {
		return err
	}
	return kptClient.InstallResourceGroup()
}

func (r *LiveReconciler) GetKptClient(ctx context.Context, live kuberikiov1alpha1.Live) (*livepkg.KptClient, error) {
	client, err := livepkg.NewKptClient(ctx, *rest.CopyConfig(r.Config))
	if err != nil {
		return nil, err
	}

	client.ImpersonateForResources(types.NamespacedName{
		Name:      live.GetServiceAccountName(),
		Namespace: live.Namespace,
	})
	return client, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	build, err := ctrl.NewControllerManagedBy(mgr).
		For(&kuberikiov1alpha1.Live{}).
		Build(r)
	if err != nil {
		return err
	}
	return build.Watch(&source.Channel{
		Source: r.KptClientEvents,
	}, &handler.EnqueueRequestForObject{})
}
