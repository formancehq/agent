package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alitto/pond"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/stretchr/testify/require"
)

// fakeHandler instruments the dispatcher under test. Every handle() call
// signals on `started`, blocks waiting for a value on `release` (or until
// its context is cancelled), and records the resulting outcome.
type fakeHandler struct {
	started chan *generated.Order
	release chan struct{}

	mu   sync.Mutex
	done []handlerOutcome
}

type handlerOutcome struct {
	order     *generated.Order
	cancelled bool
}

func newFakeHandler(buffer int) *fakeHandler {
	return &fakeHandler{
		started: make(chan *generated.Order, buffer),
		release: make(chan struct{}, buffer),
	}
}

func (h *fakeHandler) handle(ctx context.Context, order *generated.Order) {
	h.started <- order

	select {
	case <-ctx.Done():
		h.record(order, true)
		return
	case <-h.release:
	}
	h.record(order, false)
}

func (h *fakeHandler) record(order *generated.Order, cancelled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.done = append(h.done, handlerOutcome{order: order, cancelled: cancelled})
}

func (h *fakeHandler) outcomes() []handlerOutcome {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]handlerOutcome, len(h.done))
	copy(out, h.done)
	return out
}

func existingOrder(stackName string) *generated.Order {
	return &generated.Order{Message: &generated.Order_ExistingStack{
		ExistingStack: &generated.Stack{ClusterName: stackName},
	}}
}

func deletedOrder(stackName string) *generated.Order {
	return &generated.Order{Message: &generated.Order_DeletedStack{
		DeletedStack: &generated.DeletedStack{ClusterName: stackName},
	}}
}

func disabledOrder(stackName string) *generated.Order {
	return &generated.Order{Message: &generated.Order_DisabledStack{
		DisabledStack: &generated.DisabledStack{ClusterName: stackName},
	}}
}

const dispatchTimeout = 2 * time.Second

func expectStart(t *testing.T, h *fakeHandler, want *generated.Order) {
	t.Helper()
	select {
	case got := <-h.started:
		require.Same(t, want, got, "wrong order started")
	case <-time.After(dispatchTimeout):
		t.Fatalf("timed out waiting for handler to start order")
	}
}

func expectNoStart(t *testing.T, h *fakeHandler, within time.Duration) {
	t.Helper()
	select {
	case got := <-h.started:
		t.Fatalf("handler unexpectedly started order %v", got)
	case <-time.After(within):
	}
}

// TestStackDispatcher_SerializesSameStack confirms two orders for the
// same stack are processed strictly one-after-the-other.
func TestStackDispatcher_SerializesSameStack(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(8)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	o1 := existingOrder("stack-a")
	o2 := disabledOrder("stack-a") // distinct type so it is not coalesced

	d.Dispatch(context.Background(), o1)
	expectStart(t, h, o1)

	d.Dispatch(context.Background(), o2)
	expectNoStart(t, h, 50*time.Millisecond)

	h.release <- struct{}{} // unblock o1
	expectStart(t, h, o2)
	h.release <- struct{}{} // unblock o2

	require.Eventually(t, func() bool { return len(h.outcomes()) == 2 }, time.Second, 5*time.Millisecond)
	outcomes := h.outcomes()
	require.Same(t, o1, outcomes[0].order)
	require.False(t, outcomes[0].cancelled)
	require.Same(t, o2, outcomes[1].order)
	require.False(t, outcomes[1].cancelled)
}

// TestStackDispatcher_ParallelDifferentStacks confirms two orders for
// distinct stacks run concurrently.
func TestStackDispatcher_ParallelDifferentStacks(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(8)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	a := existingOrder("stack-a")
	b := existingOrder("stack-b")

	d.Dispatch(context.Background(), a)
	d.Dispatch(context.Background(), b)

	// Both should start before either is released.
	started := map[*generated.Order]bool{}
	for range 2 {
		select {
		case o := <-h.started:
			started[o] = true
		case <-time.After(dispatchTimeout):
			t.Fatalf("timed out waiting for both orders to start")
		}
	}
	require.True(t, started[a])
	require.True(t, started[b])

	h.release <- struct{}{}
	h.release <- struct{}{}

	require.Eventually(t, func() bool { return len(h.outcomes()) == 2 }, time.Second, 5*time.Millisecond)
}

// TestStackDispatcher_CoalescesConsecutiveExistingStack confirms that
// when several ExistingStack orders pile up behind an in-flight one,
// only the freshest is replayed.
func TestStackDispatcher_CoalescesConsecutiveExistingStack(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(8)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	first := existingOrder("stack-a")
	d.Dispatch(context.Background(), first)
	expectStart(t, h, first)

	stale := existingOrder("stack-a")
	stale2 := existingOrder("stack-a")
	freshest := existingOrder("stack-a")
	// They cancel `first` (Existing→Existing) but that is fine for this
	// test — `first` records cancelled=true and the loop advances.
	d.Dispatch(context.Background(), stale)
	d.Dispatch(context.Background(), stale2)
	d.Dispatch(context.Background(), freshest)

	// Drain `first` (cancelled by stale).
	require.Eventually(t, func() bool {
		o := h.outcomes()
		return len(o) >= 1 && o[0].cancelled
	}, time.Second, 5*time.Millisecond)

	// The next handler invocation should be `freshest`, not stale.
	expectStart(t, h, freshest)
	h.release <- struct{}{}

	require.Eventually(t, func() bool { return len(h.outcomes()) == 2 }, time.Second, 5*time.Millisecond)
	outcomes := h.outcomes()
	require.Same(t, first, outcomes[0].order)
	require.True(t, outcomes[0].cancelled, "first should be cancelled by the fresher order")
	require.Same(t, freshest, outcomes[1].order)
	require.False(t, outcomes[1].cancelled)
}

// TestStackDispatcher_DeleteCancelsInflightExisting confirms that an
// arriving DeletedStack interrupts an in-flight ExistingStack so the
// delete runs promptly.
func TestStackDispatcher_DeleteCancelsInflightExisting(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(8)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	existing := existingOrder("stack-a")
	del := deletedOrder("stack-a")

	d.Dispatch(context.Background(), existing)
	expectStart(t, h, existing)

	d.Dispatch(context.Background(), del)

	// Existing should be cancelled without anyone releasing it.
	expectStart(t, h, del)
	h.release <- struct{}{}

	require.Eventually(t, func() bool { return len(h.outcomes()) == 2 }, time.Second, 5*time.Millisecond)
	outcomes := h.outcomes()
	require.Same(t, existing, outcomes[0].order)
	require.True(t, outcomes[0].cancelled, "in-flight Existing should be cancelled by Delete")
	require.Same(t, del, outcomes[1].order)
	require.False(t, outcomes[1].cancelled)
}

// TestStackDispatcher_DeleteDoesNotCancelDelete asserts an in-flight
// DeletedStack is never interrupted, even by a later order.
func TestStackDispatcher_DeleteDoesNotCancelDelete(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(8)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	del := deletedOrder("stack-a")
	follower := deletedOrder("stack-a")

	d.Dispatch(context.Background(), del)
	expectStart(t, h, del)

	d.Dispatch(context.Background(), follower)
	expectNoStart(t, h, 50*time.Millisecond)

	h.release <- struct{}{}
	expectStart(t, h, follower)
	h.release <- struct{}{}

	require.Eventually(t, func() bool { return len(h.outcomes()) == 2 }, time.Second, 5*time.Millisecond)
	for _, o := range h.outcomes() {
		require.False(t, o.cancelled, "delete handlers should never be cancelled")
	}
}

// TestStackDispatcher_HandlerRespectsParentCancel makes sure the parent
// context still propagates: when the caller's ctx is done, in-flight
// handlers observe ctx.Done() and the dispatcher quiesces.
func TestStackDispatcher_HandlerRespectsParentCancel(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(4)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	ctx, cancel := context.WithCancel(context.Background())

	o := existingOrder("stack-a")
	d.Dispatch(ctx, o)
	expectStart(t, h, o)

	cancel()

	require.Eventually(t, func() bool {
		out := h.outcomes()
		return len(out) == 1 && out[0].cancelled
	}, time.Second, 5*time.Millisecond)
}

// TestShouldCancelInFlight exhaustively covers the cancellation matrix.
func TestShouldCancelInFlight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  orderType
		incoming *generated.Order
		want     bool
	}{
		{"existing supersedes existing", orderExistingStack, existingOrder("x"), true},
		{"delete supersedes existing", orderExistingStack, deletedOrder("x"), true},
		{"delete supersedes enabled", orderEnabledStack, deletedOrder("x"), true},
		{"delete supersedes disabled", orderDisabledStack, deletedOrder("x"), true},
		{"existing does not supersede delete", orderDeletedStack, existingOrder("x"), false},
		{"delete does not supersede delete", orderDeletedStack, deletedOrder("x"), false},
		{"disabled does not supersede existing", orderExistingStack, disabledOrder("x"), false},
		{"disabled does not supersede disabled", orderDisabledStack, disabledOrder("x"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldCancelInFlight(tc.current, tc.incoming))
		})
	}
}

// TestStackDispatcher_UnknownOrderBypassesMailbox asserts that orders
// without a stack scope are still processed (today only stack-scoped
// orders reach the listener, but this defends against future variants).
func TestStackDispatcher_UnknownOrderBypassesMailbox(t *testing.T) {
	t.Parallel()
	h := newFakeHandler(2)
	wp := pond.New(5, 5)
	defer wp.StopAndWait()
	d := newStackDispatcher(wp, h.handle)

	// Connected is a non-stack-scoped Order variant.
	o := &generated.Order{Message: &generated.Order_Connected{Connected: &generated.Connected{}}}
	d.Dispatch(context.Background(), o)
	expectStart(t, h, o)
	h.release <- struct{}{}

	require.Eventually(t, func() bool { return len(h.outcomes()) == 1 }, time.Second, 5*time.Millisecond)

	// Sanity: stackNameFromOrder returned empty so no queue should have been created.
	d.mu.Lock()
	require.Empty(t, d.queues)
	d.mu.Unlock()
}
