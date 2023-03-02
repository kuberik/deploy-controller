package v1alpha1

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	//+kubebuilder:scaffold:imports
)

var (
	trueVal  = true
	falseVal = false
)

var _ = Describe("Live webhook", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		LiveNamespace = "default"
		LivePath      = "."

		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)

	Context("Interrupts on Live", func() {
		testCaseCounter := 0
		var liveLookupKey *types.NamespacedName
		var interruptible *bool

		JustBeforeEach(func() {
			testCaseCounter++

			liveLookupKey = &types.NamespacedName{Name: fmt.Sprintf("interrupt-%d", testCaseCounter), Namespace: LiveNamespace}

			By("By creating a new Live")
			ctx := context.Background()
			live := &Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      liveLookupKey.Name,
					Namespace: liveLookupKey.Namespace,
				},
				Spec: LiveSpec{
					Path:          LivePath,
					Interruptible: *interruptible,
					Repository: Repository{
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
		})

		When("Live is not interruptible", func() {
			BeforeEach(func() {
				interruptible = &falseVal
			})

			It("Should allow Live updates before apply phase", func() {
				live := &Live{}

				By("By trying to update the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})

			It("Should allow updates when reconciling Live and interruptible is set to true on new version", func() {
				live := &Live{}

				By("By trying to update the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Interruptible = true
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})

			It("Should disallow updates when Live started applying resources", func() {
				live := &Live{}

				By("By updating the Live status to applying")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.SetPhase(LivePhase{Name: LivePhaseApplying})
				Expect(k8sClient.Status().Update(ctx, live)).Should(Succeed())

				By("By updating the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).ShouldNot(Succeed())
			})

			It("Should allow updates when Live completed reconciling", func() {
				live := &Live{}

				By("By updating the Live status to complete")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.SetPhase(LivePhase{Name: LivePhaseSucceeded})
				Expect(k8sClient.Status().Update(ctx, live)).Should(Succeed())

				By("By updating the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})
		})

		When("Live is interruptible", func() {
			BeforeEach(func() {
				interruptible = &trueVal
			})

			It("Should allow Live updates before apply phase", func() {
				live := &Live{}

				By("By trying to update the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})

			It("Should allow updates when Live started applying resources", func() {
				live := &Live{}

				By("By updating the Live status to applying")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.SetPhase(LivePhase{Name: LivePhaseApplying})
				Expect(k8sClient.Status().Update(ctx, live)).Should(Succeed())

				By("By updating the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})

			It("Should allow updates when Live completed reconciling", func() {
				live := &Live{}

				By("By updating the Live status to complete")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.SetPhase(LivePhase{Name: LivePhaseSucceeded})
				Expect(k8sClient.Status().Update(ctx, live)).Should(Succeed())

				By("By updating the Live spec")
				Expect(k8sClient.Get(ctx, *liveLookupKey, live)).Should(Succeed())
				live.Spec.Commit = plumbing.NewHash("1234567890").String()
				Expect(k8sClient.Update(ctx, live)).Should(Succeed())
			})
		})
	})

	Context("Live ServiceAccountName update", func() {
		It("Should deny updating Live's ServiceAccountName", func() {
			liveLookupKey := &types.NamespacedName{Name: "service-account-update", Namespace: LiveNamespace}

			By("By creating a new Live")
			ctx := context.Background()
			live := &Live{
				ObjectMeta: metav1.ObjectMeta{
					Name:      liveLookupKey.Name,
					Namespace: liveLookupKey.Namespace,
				},
				Spec: LiveSpec{
					ServiceAccountName: "test",
					Path:               LivePath,
					Repository: Repository{
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

			By("Updating Live's ServiceAcount")
			Consistently(func() error {
				live.Spec.ServiceAccountName = "changed"
				return k8sClient.Update(ctx, live)
			}, timeout, interval).Should(HaveOccurred())
		})
	})
})
