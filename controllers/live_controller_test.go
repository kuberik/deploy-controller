package controllers

import (
	"context"
	"fmt"
	"io/fs"
	"testing/fstest"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rgfilev1alpha1 "github.com/GoogleContainerTools/kpt/pkg/api/resourcegroup/v1alpha1"
	kuberikiov1alpha1 "github.com/kuberik/kuberik/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	//+kubebuilder:scaffold:imports
)

func commitWithDefaults(worktree *git.Worktree) (plumbing.Hash, error) {
	return worktree.Commit("Commit message", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})
}

func generateGitRepository(repoDir string, filesystem fstest.MapFS) (*git.Repository, error) {
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		return nil, err
	}

	config, err := repo.Config()
	if err != nil {
		return nil, err
	}

	config.Raw.Sections = append(config.Raw.Sections, &gitconfig.Section{
		Name: "uploadpack",
		Options: []*gitconfig.Option{
			{Key: "allowReachableSHA1InWant", Value: "true"},
		},
	})
	if err := repo.SetConfig(config); err != nil {
		return nil, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	err = fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return worktree.Filesystem.MkdirAll(path, 0777)
		}

		gitFile, err := worktree.Filesystem.Create(path)
		if err != nil {
			return err
		}

		fileContents, err := filesystem.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := gitFile.Write(fileContents); err != nil {
			return err
		}

		_, err = worktree.Add(path)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "John Doe",
			Email: "john@doe.org",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, err
	}

	return repo, nil
}

var _ = Describe("Live controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		LivePath = "."

		timeout             = time.Second * 20
		consistentlyTimeout = time.Second * 5
		interval            = time.Millisecond * 100
	)

	assertLiveReadyStatus := func(liveLookupKey types.NamespacedName, reason string, status metav1.ConditionStatus, consistently bool) {
		check := func(check func(actual interface{}, intervals ...interface{}) AsyncAssertion, intervals ...interface{}) {
			check(func() (*kuberikiov1alpha1.Live, error) {
				live := &kuberikiov1alpha1.Live{}
				err := k8sClient.Get(ctx, liveLookupKey, live)
				if err != nil {
					return nil, err
				}
				return live, nil
			}, intervals...).Should(SatisfyAll(
				WithTransform(func(live *kuberikiov1alpha1.Live) bool {
					readyCondition := live.GetReadyCondition()
					if readyCondition == nil {
						return false
					}
					return readyCondition.ObservedGeneration == live.Generation
				}, BeTrue()),
				WithTransform(func(live *kuberikiov1alpha1.Live) *metav1.Condition {
					return live.GetReadyCondition()
				}, SatisfyAll(
					HaveField("Type", "Ready"),
					HaveField("Reason", reason),
				)),
			))
		}
		check(Eventually, timeout, interval)
		if consistently {
			check(Consistently, consistentlyTimeout, interval)
		}

		Eventually(func() error {
			_, err := dynClient.Resource(schema.GroupVersionResource{
				Group:    rgfilev1alpha1.RGFileGroup,
				Version:  rgfilev1alpha1.RGFileVersion,
				Resource: "resourcegroups",
			}).Namespace(liveLookupKey.Namespace).Get(ctx, liveLookupKey.Name, metav1.GetOptions{})
			return err
		}, timeout, interval).Should(Succeed())
	}

	Context("Creating or updating a Live with a Pod", func() {
		var liveLookupKey *types.NamespacedName
		var gitFiles fstest.MapFS
		var transformers string
		var commit plumbing.Hash
		var repo *git.Repository
		testCaseCounter := 0

		setPodPhasePending := func(podLookupKey types.NamespacedName) {
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, podLookupKey, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodPending
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{}
			Eventually(func() error {
				return k8sClient.Status().Update(ctx, pod)
			}, timeout, interval).Should(Succeed())
		}

		setPodPhaseCrashed := func(podLookupKey types.NamespacedName, containerName string) {
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, podLookupKey, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodRunning
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
				Name: containerName,
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: rand.String(10),
					},
				},
			}}
			Eventually(func() error {
				return k8sClient.Status().Update(ctx, pod)
			}, timeout, interval).Should(Succeed())
		}

		setPodPhaseComplete := func(podLookupKey types.NamespacedName) {
			pod := &corev1.Pod{}

			Eventually(func() error {
				return k8sClient.Get(ctx, podLookupKey, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodSucceeded
			Expect(k8sClient.Status().Update(ctx, pod)).Should(Succeed())
		}

		JustBeforeEach(func() {
			testCaseCounter++

			Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-sa-cluster-admin",
					Namespace: "default",
				},
				Subjects: []rbacv1.Subject{{
					Name: "default",
					Kind: rbacv1.ServiceAccountKind,
				}},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "cluster-admin",
				},
			}, &client.CreateOptions{})).Should(Or(
				Succeed(),
				WithTransform(func(err error) bool { return errors.IsAlreadyExists(err) }, BeTrue()),
			))

			By("By creating a git repository with kustomize layer")
			repoDir := GinkgoT().TempDir()
			var err error
			repo, err = generateGitRepository(repoDir, gitFiles)
			Expect(err).NotTo(HaveOccurred())

			By("By creating a new Live")
			ctx := context.Background()
			head, err := repo.Head()
			commit = head.Hash()
			Expect(err).NotTo(HaveOccurred())

			live := &kuberikiov1alpha1.Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("live-test-%d", testCaseCounter),
					Namespace: "default",
				},
				Spec: kuberikiov1alpha1.LiveSpec{
					Path: LivePath,
					Repository: kuberikiov1alpha1.Repository{
						URL: fmt.Sprintf("file://%s", repoDir),
					},
					Commit:       commit.String(),
					Transformers: transformers,
				},
			}
			Expect(k8sClient.Create(ctx, live)).Should(Succeed())

			liveLookupKey = &types.NamespacedName{Name: live.Name, Namespace: live.Namespace}
			createdLive := &kuberikiov1alpha1.Live{}

			// We'll need to retry getting this newly created resource, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, *liveLookupKey, createdLive)
			}, timeout, interval).Should(Succeed())
		})

		When("Deployed resources reconcile successfully", func() {
			BeforeEach(func() {
				gitFiles = fstest.MapFS{
					"pod.yaml": {
						Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: live-pod-success
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx
`)},
					"kustomization.yaml": {
						Data: []byte(`
resources: [pod.yaml]
`)},
				}
			})
			It("Should reconcile Live successfully", func() {
				podLookupKey := types.NamespacedName{Name: "live-pod-success", Namespace: liveLookupKey.Namespace}

				By("By waiting for Pod to reconcile")
				setPodPhaseComplete(podLookupKey)
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)
			})
		})
		When("Deployed resources reconcile fails", func() {
			BeforeEach(func() {
				gitFiles = fstest.MapFS{
					"pod.yaml": {
						Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: live-pod-failure
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx
`)},
					"kustomization.yaml": {
						Data: []byte(`
resources: [pod.yaml]
`)},
				}
			})
			It("Should reconcile Live with a failure", func() {
				podLookupKey := types.NamespacedName{Name: "live-pod-failure", Namespace: liveLookupKey.Namespace}
				containerName := "nginx"

				stop := make(chan interface{})
				// simulate repeatedly crashing pod
				go func() {
					for {
						select {
						case <-time.After(time.Second):
							setPodPhasePending(podLookupKey)
							setPodPhaseCrashed(podLookupKey, containerName)
						case <-stop:
							return
						}
					}
				}()

				By("By waiting for Pod to try to reconcile")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseApplying), metav1.ConditionFalse, false)

				By("By waiting for Pod to crash")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseFailed), metav1.ConditionFalse, false)

				By("By waiting for Pod to try to reconcile again")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseApplying), metav1.ConditionFalse, false)

				By("By waiting for Pod to crash again")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseFailed), metav1.ConditionFalse, false)

				stop <- nil
				close(stop)
			})
		})
		When("Deployed resources reconcile fails on first try", func() {
			BeforeEach(func() {
				gitFiles = fstest.MapFS{
					"resources.yaml": {
						Data: []byte(`
        apiVersion: v1
        kind: Pod
        metadata:
          name: live-pod-recover
          namespace: default
        spec:
          containers:
          - name: nginx
            image: nginx
        `)},
					"kustomization.yaml": {
						Data: []byte(`
        resources: [resources.yaml]
        `)},
				}
			})
			It("Should reconcile Live with a failure and then success", func() {
				podLookupKey := types.NamespacedName{Name: "live-pod-recover", Namespace: liveLookupKey.Namespace}
				containerName := "nginx"

				By("By waiting for Pod to try to reconcile")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseApplying), metav1.ConditionFalse, false)

				By("By waiting for Pod to crash")
				setPodPhaseCrashed(podLookupKey, containerName)
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseFailed), metav1.ConditionFalse, false)

				By("By waiting for Pod to try to reconcile again")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseApplying), metav1.ConditionFalse, false)

				By("By waiting for Pod to finish with success")
				setPodPhaseComplete(podLookupKey)
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)
			})
		})
		When("Live transformers are set", func() {
			BeforeEach(func() {
				gitFiles = fstest.MapFS{
					"pod.yaml": {
						Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: live-pod-transformed
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx:latest
`)},
					"kustomization.yaml": {
						Data: []byte(`
resources: [pod.yaml]
`)},
					"transform/kustomization.yaml": {
						Data: []byte(`
resources: [replace.yaml]
`)},
					"transform/replace.yaml": {
						Data: []byte(`
apiVersion: builtin
kind: ReplacementTransformer
metadata:
  name: notImportantHere
replacements:
- source:
    kind: Live
    fieldPath: spec.commit
  targets:
  - select:
      kind: Pod
    fieldPaths:
    - spec.containers.[name=nginx].image
    options:
      delimiter: ":"
      index: 1
`)},
				}
				transformers = "transform"
			})
			It("Should reconcile Live successfully with transformer image tag", func() {
				podLookupKey := types.NamespacedName{Name: "live-pod-transformed", Namespace: liveLookupKey.Namespace}
				createdPod := &corev1.Pod{}

				By("By waiting for Pod to reconcile")
				Eventually(func() error {
					return k8sClient.Get(ctx, podLookupKey, createdPod)
				}, timeout, interval).Should(Succeed())
				Expect(createdPod.Spec.Containers[0].Image).Should(Equal(fmt.Sprintf("nginx:%s", commit)))

				createdPod.Status.Phase = corev1.PodSucceeded
				Expect(k8sClient.Status().Update(ctx, createdPod)).Should(Succeed())
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)
			})
		})
		When("Live commit is updated", func() {
			BeforeEach(func() {
				gitFiles = fstest.MapFS{
					"kustomization.yaml": {
						Data: []byte(`
configMapGenerator:
- name: live-updated-commit
  namespace: default
  literals:
  - foo=bar
  options:
    disableNameSuffixHash: true
`)},
				}
			})
			It("Should update the deployed resources", func() {
				configMapLookupKey := types.NamespacedName{Name: "live-updated-commit", Namespace: liveLookupKey.Namespace}
				createdConfigMap := &corev1.ConfigMap{}

				By("By waiting for Live to reconcile first commit")
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)

				Eventually(func() error {
					return k8sClient.Get(ctx, configMapLookupKey, createdConfigMap)
				}, timeout, interval).Should(Succeed())
				Expect(createdConfigMap.Data["foo"]).To(Equal("bar"))

				By("Updating the Live commit")
				worktree, err := repo.Worktree()
				Expect(err).NotTo(HaveOccurred())

				kustomizationFilePath := "kustomization.yaml"
				kustomizationFile, err := worktree.Filesystem.Create(kustomizationFilePath)
				Expect(err).NotTo(HaveOccurred())
				_, err = kustomizationFile.Write([]byte(`
configMapGenerator:
- name: live-updated-commit
  namespace: default
  literals:
  - foo=bar2
  options:
    disableNameSuffixHash: true
`))
				Expect(err).NotTo(HaveOccurred())
				_, err = worktree.Add(kustomizationFilePath)
				Expect(err).NotTo(HaveOccurred())
				newCommit, err := commitWithDefaults(worktree)
				Expect(err).NotTo(HaveOccurred())

				createdLive := &kuberikiov1alpha1.Live{}
				Expect(k8sClient.Get(ctx, *liveLookupKey, createdLive)).Should(Succeed())

				createdLive.Spec.Commit = newCommit.String()
				Expect(k8sClient.Update(ctx, createdLive)).Should(Succeed())

				By("By waiting for Live to reconcile second commit")

				Eventually(func() (string, error) {
					if err := k8sClient.Get(ctx, configMapLookupKey, createdConfigMap); err != nil {
						return "", err
					}
					return createdConfigMap.Data["foo"], nil
				}, timeout, interval).Should(Equal("bar2"))
				assertLiveReadyStatus(*liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)
			})
		})
		AfterEach(func() {
			transformers = ""
		})
	})

	Context("Creating or updating a Live with reference to a private repository", func() {
		It("Should deploy the objects from private repository", func() {
			authSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "private-repo-auth",
					Namespace: "default",
				},
				StringData: map[string]string{
					"token": getGithubTokenOrSkip(),
				},
			}
			Expect(k8sClient.Create(ctx, &authSecret)).Should(Succeed())

			live := &kuberikiov1alpha1.Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "private-repo",
					Namespace: "default",
				},
				Spec: kuberikiov1alpha1.LiveSpec{
					Path: LivePath,
					Repository: kuberikiov1alpha1.Repository{
						URL: "https://github.com/kuberik/git-auth-kustomize-test.git",
						Auth: &kuberikiov1alpha1.RepositoryAuth{
							SecretRef: corev1.LocalObjectReference{
								Name: authSecret.Name,
							},
						},
					},
					Commit: "c984d9d19a53160658b0b70a326586ca3dc66874",
				},
			}
			Expect(k8sClient.Create(ctx, live)).Should(Succeed())

			liveLookupKey := types.NamespacedName{Name: live.Name, Namespace: live.Namespace}
			assertLiveReadyStatus(liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)
		})
	})

	Context("Deleting a Live", func() {
		It("Should clean up all deployed resources", func() {
			By("Waiting for Live to reconcile")
			configMapLookupKey := types.NamespacedName{Name: "cleanup-resources", Namespace: "default"}
			gitFiles := fstest.MapFS{
				"kustomization.yaml": {
					Data: []byte(fmt.Sprintf(`
configMapGenerator:
- name: %s
  namespace: default
  options:
    disableNameSuffixHash: true
`, configMapLookupKey.Name))},
			}
			repoDir := GinkgoT().TempDir()
			repo, err := generateGitRepository(repoDir, gitFiles)
			Expect(err).NotTo(HaveOccurred())

			ctx := context.Background()
			head, err := repo.Head()
			Expect(err).NotTo(HaveOccurred())

			live := &kuberikiov1alpha1.Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "live-delete-cleanup",
					Namespace: "default",
				},
				Spec: kuberikiov1alpha1.LiveSpec{
					Path: LivePath,
					Repository: kuberikiov1alpha1.Repository{
						URL: fmt.Sprintf("file://%s", repoDir),
					},
					Commit: head.Hash().String(),
				},
			}
			Expect(k8sClient.Create(ctx, live)).Should(Succeed())

			liveLookupKey := types.NamespacedName{Name: live.Name, Namespace: live.Namespace}
			assertLiveReadyStatus(liveLookupKey, string(kuberikiov1alpha1.LivePhaseSucceeded), metav1.ConditionTrue, true)

			getConfigMap := func() error {
				return k8sClient.Get(ctx, configMapLookupKey, &corev1.ConfigMap{})
			}
			getLive := func() error {
				return k8sClient.Get(ctx, liveLookupKey, &kuberikiov1alpha1.Live{})
			}

			Expect(getLive()).To(Succeed())
			Expect(getConfigMap()).To(Succeed())

			By("Deleting Live")
			Expect(k8sClient.Delete(ctx, live)).To(Succeed())

			By("Cleaning up resources")
			Eventually(func() bool {
				return errors.IsNotFound(getConfigMap())
			}).Should(BeTrue())

			By("Removing finalizers so that Live gets deleted")
			Eventually(func() bool {
				return errors.IsNotFound(getLive())
			}).Should(BeTrue())
		})
	})

	Context("Ensure finalizer is set", func() {
		It("Should set it when Live is created/updated", func() {
			liveLookupKey := &types.NamespacedName{Name: "finalizer-create", Namespace: "default"}

			By("By creating a new Live")
			ctx := context.Background()
			live := &kuberikiov1alpha1.Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      liveLookupKey.Name,
					Namespace: liveLookupKey.Namespace,
				},
				Spec: kuberikiov1alpha1.LiveSpec{
					Path:          LivePath,
					Interruptible: true,
					Repository: kuberikiov1alpha1.Repository{
						URL: "https://github.com/kuberik/kuberik",
					},
					Commit: plumbing.ZeroHash.String(),
				},
			}
			Expect(k8sClient.Create(ctx, live)).Should(Succeed())

			// We'll need to retry getting this newly created resource, given that creation may not immediately happen.
			Eventually(func() error {
				return k8sClient.Get(ctx, *liveLookupKey, live)
			}, timeout, interval).Should(Succeed())

			Eventually(func() []string {
				k8sClient.Get(ctx, *liveLookupKey, live)
				return live.Finalizers
			}).Should(SatisfyAll(ContainElement("kuberik.io/live-destroy"), HaveLen(1)))
		})
	})
})
