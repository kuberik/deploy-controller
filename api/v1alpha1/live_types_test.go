package v1alpha1

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLiveBackoffRemaining(t *testing.T) {
	live := Live{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
	}

	live.SetPhase(LivePhase{Name: LivePhaseSucceeded})
	currentTime := time.Now()
	live.GetReadyCondition().LastTransitionTime = metav1.NewTime(currentTime)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Second*0)
	live.Generation += 1

	live.SetPhase(LivePhase{Name: LivePhaseApplying})
	live.SetPhase(LivePhase{Name: LivePhaseFailed})
	currentTime = currentTime.Add(time.Millisecond * 350)
	live.GetReadyCondition().LastTransitionTime = metav1.NewTime(currentTime)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Millisecond*2000)

	live.SetPhase(LivePhase{Name: LivePhaseApplying})
	live.SetPhase(LivePhase{Name: LivePhaseFailed})
	currentTime = currentTime.Add(time.Millisecond * 350)
	live.GetReadyCondition().LastTransitionTime = metav1.NewTime(currentTime)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Millisecond*4000)

	currentTime = currentTime.Add(time.Millisecond * 500)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Millisecond*3500)

	// Go past the backoff expiry time to make sure the remaining backoff time is at 0
	currentTime = currentTime.Add(time.Millisecond * 10000)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Millisecond*0)
	// Add some extra time to make sure we didn't hit the right spot with the previous test
	currentTime = currentTime.Add(time.Millisecond * 10)
	assert.Equal(t, live.backoffRemainingAt(currentTime), time.Millisecond*0)
}
