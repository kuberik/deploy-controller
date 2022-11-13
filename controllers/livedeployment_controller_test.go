package controllers

import (
	"context"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	fixtures "github.com/go-git/go-git-fixtures/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("LiveDeployment controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating/updating a LiveDeployment", func() {
		const (
			LiveDeploymentName      = "ldc-test-01"
			LiveDeploymentNamespace = "default"

			LiveName      = LiveDeploymentName
			LiveNamespace = LiveDeploymentNamespace

			LivePath       = "dummy/path"
			LiveRepoCommit = "e8d3ffab552895c19b9fcf7aa264d277cde33881"
			LiveRepoBranch = "branch"
		)
		It("Should create/update the Live resource", func() {
			By("By creating a git repository")
			repoURL := fixtures.Basic().One().DotGit().Root()

			By("By creating a new LiveDeployment")
			ctx := context.Background()
			liveDeployment := &kuberikiov1alpha1.LiveDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      LiveDeploymentName,
					Namespace: LiveDeploymentNamespace,
				},
				Spec: kuberikiov1alpha1.LiveDeploymentSpec{
					Branch: LiveRepoBranch,
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

			liveDeploymentLookupKey := types.NamespacedName{Name: LiveDeploymentName, Namespace: LiveDeploymentNamespace}
			createdLiveDeployment := &kuberikiov1alpha1.LiveDeployment{}

			// We'll need to retry getting this newly created resource, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, liveDeploymentLookupKey, createdLiveDeployment)
			}, timeout, interval).Should(Succeed())

			By("By creating the Live resource")
			Eventually(func() (kuberikiov1alpha1.Live, error) {
				liveLookupKey := types.NamespacedName{Name: LiveName, Namespace: LiveNamespace}
				createdLive := &kuberikiov1alpha1.Live{}
				err := k8sClient.Get(ctx, liveLookupKey, createdLive)
				if err != nil {
					return *createdLive, err
				}

				return *createdLive, nil
			}, timeout, interval).Should(SatisfyAll(
				WithTransform(func(live kuberikiov1alpha1.Live) metav1.OwnerReference { return live.OwnerReferences[0] }, SatisfyAll(
					WithTransform(func(owner metav1.OwnerReference) types.UID { return owner.UID }, Equal(createdLiveDeployment.UID)),
					WithTransform(func(owner metav1.OwnerReference) bool { return *owner.Controller }, BeTrue()),
				)),
				WithTransform(func(live kuberikiov1alpha1.Live) (kuberikiov1alpha1.LiveSpec, error) { return live.Spec, nil }, SatisfyAll(
					HaveField("Path", LivePath),
					HaveField("Commit", LiveRepoCommit),
					WithTransform(func(spec kuberikiov1alpha1.LiveSpec) kuberikiov1alpha1.Repository { return spec.Repository }, SatisfyAll(
						HaveField("URL", createdLiveDeployment.Spec.Template.Spec.Repository.URL),
					)),
				)),
			), "LiveDeployment %s should create Live %v", LiveDeploymentName, LiveName)

			By("Creating a new commit on the branch")
			fs := memfs.New()
			inMemoryStorage := memory.NewStorage()
			repo, err := git.Clone(inMemoryStorage, fs, &git.CloneOptions{
				URL:           repoURL,
				ReferenceName: plumbing.NewBranchReferenceName(LiveRepoBranch),
				SingleBranch:  true,
			})
			Expect(err).Should(Succeed())

			worktree, err := repo.Worktree()
			Expect(err).Should(Succeed())
			exampleAddedFile := "example-git-file"
			file, err := fs.Create(exampleAddedFile)
			Expect(err).Should(Succeed())
			_, err = file.Write([]byte("example-git-file-content"))
			Expect(err).Should(Succeed())
			_, err = worktree.Add(exampleAddedFile)
			Expect(err).Should(Succeed())

			newCommit, err := worktree.Commit("example go-git commit", &git.CommitOptions{
				Author: &object.Signature{
					Name:  "John Doe",
					Email: "john@doe.org",
					When:  time.Now(),
				},
			})
			Expect(err).Should(Succeed())
			Expect(repo.Push(&git.PushOptions{})).Should(Succeed())

			By("Updating the Live Resource")
			Eventually(func() (kuberikiov1alpha1.Live, error) {
				liveLookupKey := types.NamespacedName{Name: LiveName, Namespace: LiveNamespace}
				createdLive := &kuberikiov1alpha1.Live{}
				err := k8sClient.Get(ctx, liveLookupKey, createdLive)
				if err != nil {
					return *createdLive, err
				}

				return *createdLive, nil
			}, timeout, interval).Should(
				WithTransform(func(live kuberikiov1alpha1.Live) (kuberikiov1alpha1.LiveSpec, error) { return live.Spec, nil }, SatisfyAll(
					HaveField("Commit", newCommit.String()),
				)),
			)
		})
	})

	Context("When creating a LiveDeployment referencing private repo", func() {
		It("Should create/update the Live resource", func() {
			ctx := context.Background()

			By("By creating a new LiveDeployment")
			authSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ld-private-repo-auth",
					Namespace: "default",
				},
				StringData: map[string]string{
					"token": getGithubTokenOrSkip(),
				},
			}
			Expect(k8sClient.Create(ctx, &authSecret)).Should(Succeed())

			liveDeployment := &kuberikiov1alpha1.LiveDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "private-repo-live-deployment",
					Namespace: "default",
				},
				Spec: kuberikiov1alpha1.LiveDeploymentSpec{
					Branch: "main",
					Template: &kuberikiov1alpha1.LiveTemplate{
						Spec: kuberikiov1alpha1.LiveSpec{
							Path: ".",
							Repository: kuberikiov1alpha1.Repository{
								URL: "https://github.com/kuberik/git-auth-kustomize-test.git",
								Auth: &kuberikiov1alpha1.RepositoryAuth{
									SecretRef: corev1.LocalObjectReference{
										Name: authSecret.Name,
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, liveDeployment)).Should(Succeed())

			liveDeploymentLookupKey := types.NamespacedName{Name: liveDeployment.Name, Namespace: liveDeployment.Namespace}
			createdLiveDeployment := &kuberikiov1alpha1.LiveDeployment{}

			// We'll need to retry getting this newly created resource, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, liveDeploymentLookupKey, createdLiveDeployment)
			}, timeout, interval).Should(Succeed())

			By("By creating the Live resource")
			Eventually(func() (kuberikiov1alpha1.Live, error) {
				liveLookupKey := types.NamespacedName{Name: liveDeploymentLookupKey.Name, Namespace: liveDeploymentLookupKey.Namespace}
				createdLive := &kuberikiov1alpha1.Live{}
				err := k8sClient.Get(ctx, liveLookupKey, createdLive)
				if err != nil {
					return *createdLive, err
				}

				return *createdLive, nil
			}, timeout, interval).Should(SatisfyAll(
				WithTransform(func(live kuberikiov1alpha1.Live) metav1.OwnerReference { return live.OwnerReferences[0] }, SatisfyAll(
					WithTransform(func(owner metav1.OwnerReference) types.UID { return owner.UID }, Equal(createdLiveDeployment.UID)),
					WithTransform(func(owner metav1.OwnerReference) bool { return *owner.Controller }, BeTrue()),
				)),
				WithTransform(func(live kuberikiov1alpha1.Live) (kuberikiov1alpha1.LiveSpec, error) { return live.Spec, nil }, SatisfyAll(
					HaveField("Path", "."),
					HaveField("Commit", "c984d9d19a53160658b0b70a326586ca3dc66874"),
					WithTransform(func(spec kuberikiov1alpha1.LiveSpec) kuberikiov1alpha1.Repository { return spec.Repository }, SatisfyAll(
						HaveField("URL", createdLiveDeployment.Spec.Template.Spec.Repository.URL),
					)),
				)),
			), "LiveDeployment %s should create Live", liveDeployment.Name)
		})
	})
})
