package controllers

import (
	"context"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	fixtures "github.com/go-git/go-git-fixtures/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("LiveDeploymentGroup controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating/updating a LiveDeploymentGroup", func() {
		const (
			LiveDeploymentGroupName      = "ldg-test-01"
			LiveDeploymentGroupNamespace = "default"
		)
		It("Should create/update/delete the LiveDeployment resources based on the branches matched", func() {
			By("By creating a git repository")
			repoURL := fixtures.Basic().One().DotGit().Root()

			By("By creating a new LiveDeploymentGroup")
			ctx := context.Background()
			liveDeployment := &kuberikiov1alpha1.LiveDeploymentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LiveDeploymentGroupName,
					Namespace: LiveDeploymentGroupNamespace,
				},
				Spec: kuberikiov1alpha1.LiveDeploymentGroupSpec{
					BranchMatch: "",
					Template: &kuberikiov1alpha1.LiveTemplate{
						Spec: kuberikiov1alpha1.LiveSpec{
							Path: "dummy/path",
							Repository: kuberikiov1alpha1.Repository{
								URL: repoURL,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, liveDeployment)).Should(Succeed())

			liveDeploymentGroupLookupKey := types.NamespacedName{Name: LiveDeploymentGroupName, Namespace: LiveDeploymentGroupNamespace}
			createdLiveDeploymentGroup := &kuberikiov1alpha1.LiveDeploymentGroup{}

			// We'll need to retry getting this newly created resource, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, liveDeploymentGroupLookupKey, createdLiveDeploymentGroup)
			}, timeout, interval).Should(Succeed())

			By("By creating the LiveDeployment resources")
			Eventually(func() ([]kuberikiov1alpha1.LiveDeployment, error) {
				createdLiveDeployments := &kuberikiov1alpha1.LiveDeploymentList{}
				err := k8sClient.List(ctx, createdLiveDeployments, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(map[string]string{
						"kuberik.io/live-deployment-group": LiveDeploymentGroupName,
					}),
				})
				if err != nil {
					return nil, err
				}

				return createdLiveDeployments.Items, nil
			}, timeout, interval).Should(SatisfyAll(
				HaveEach(HaveField("Spec.Template", createdLiveDeploymentGroup.Spec.Template)),
				WithTransform(func(lds []kuberikiov1alpha1.LiveDeployment) []string {
					branches := []string{}
					for _, ld := range lds {
						branches = append(branches, ld.Spec.Branch)
					}
					return branches
				}, ContainElements("master", "branch")),
				HaveEach(WithTransform(func(live kuberikiov1alpha1.LiveDeployment) metav1.OwnerReference { return live.OwnerReferences[0] }, SatisfyAll(
					WithTransform(func(owner metav1.OwnerReference) types.UID { return owner.UID }, Equal(createdLiveDeploymentGroup.UID)),
					WithTransform(func(owner metav1.OwnerReference) bool { return *owner.Controller }, BeTrue()),
				))),
				HaveLen(2),
			), "LiveDeploymentGroup %s should create LiveDeployment for each matched branch", LiveDeploymentGroupName)

			By("Deleting a branch")
			fs := memfs.New()
			inMemoryStorage := memory.NewStorage()
			repo, err := git.Clone(inMemoryStorage, fs, &git.CloneOptions{
				URL: repoURL,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(repo.Push(&git.PushOptions{
				Prune: true,
				RefSpecs: []config.RefSpec{
					":refs/heads/branch",
				},
			})).Should(Succeed())

			By("Deleting corresponding LiveDeployment")
			Eventually(func() ([]kuberikiov1alpha1.LiveDeployment, error) {
				createdLiveDeployments := &kuberikiov1alpha1.LiveDeploymentList{}
				err := k8sClient.List(ctx, createdLiveDeployments, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(map[string]string{
						"kuberik.io/live-deployment-group": LiveDeploymentGroupName,
					}),
				})
				if err != nil {
					return nil, err
				}

				return createdLiveDeployments.Items, nil
			}, timeout, interval).Should(SatisfyAll(
				HaveLen(1),
				WithTransform(func(lds []kuberikiov1alpha1.LiveDeployment) []string {
					branches := []string{}
					for _, ld := range lds {
						branches = append(branches, ld.Spec.Branch)
					}
					return branches
				}, ContainElements("master")),
			), "LiveDeploymentGroup %s should prune LiveDeployments for each deleted branch", LiveDeploymentGroupName)
		})
	})
})
