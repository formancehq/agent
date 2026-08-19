package internal

import (
	"context"
	"sync"

	"github.com/alitto/pond"
	"github.com/formancehq/stack/components/agent/internal/generated"
)

// orderHandler processes a single order. Implementations must honor ctx
// cancellation: when ctx is Done, return as soon as it is safe to do so.
// A partially-applied order is fine — the next order for the same stack
// will reconcile the cluster to its own desired state.
type orderHandler func(ctx context.Context, order *generated.Order)

// stackDispatcher routes membership orders through a per-stack mailbox.
//
// # Why this exists
//
// Orders from membership are stack-scoped (ExistingStack, DeletedStack,
// EnabledStack, DisabledStack). Each handler reads current K8s state
// and writes back desired state. When two orders for the same stack
// run concurrently, the slower one's write lands last and silently
// overwrites the faster one. Submitting every order to a shared worker
// pool — the previous behaviour — makes this race trivially
// reproducible whenever membership emits two orders in close succession.
//
// # Guarantees
//
//   - Per-stack FIFO: at most one order per stack is in flight at any
//     time. Pending orders for the same stack run in arrival order.
//   - Cross-stack parallelism: orders for distinct stacks remain
//     concurrent, bounded by the underlying worker pool.
//   - Cancellation on supersession: an arriving order may cancel the
//     in-flight one when continuing it would be wasteful or wrong.
//     Today two cases trigger cancellation:
//     (a) a fresher ExistingStack supersedes an in-flight ExistingStack
//     (the older snapshot is stale; replaying it would overwrite the
//     newer state),
//     (b) a DeletedStack supersedes any non-Deleted in-flight order
//     (no point creating resources we are about to tear down).
//     A DeletedStack in flight is never cancelled. Cancellation only
//     fires when a successor is already queued, so the cluster is
//     guaranteed to be reconciled by the next order.
//
// # Coalescing
//
// Consecutive ExistingStack entries in the pending queue are collapsed
// to the latest one — each carries the full desired state, so older
// snapshots are pure waste.
type stackDispatcher struct {
	wp     *pond.WorkerPool
	handle orderHandler

	mu     sync.Mutex
	queues map[string]*stackQueue
}

// stackQueue tracks the per-stack mailbox state. While running is true,
// cancel is non-nil and refers to the in-flight handler's context.
type stackQueue struct {
	running     bool
	cancel      context.CancelFunc
	currentType orderType
	pending     []*generated.Order
}

func newStackDispatcher(wp *pond.WorkerPool, handle orderHandler) *stackDispatcher {
	return &stackDispatcher{
		wp:     wp,
		handle: handle,
		queues: make(map[string]*stackQueue),
	}
}

// Dispatch routes an order through the per-stack mailbox. Orders that
// are not stack-scoped (an unrecognised message variant) bypass the
// mailbox and go straight to the pool.
func (d *stackDispatcher) Dispatch(ctx context.Context, order *generated.Order) {
	stackName := stackNameFromOrder(order)
	if stackName == "" {
		d.wp.Submit(func() { d.handle(ctx, order) })
		return
	}

	d.mu.Lock()
	q, ok := d.queues[stackName]
	if !ok {
		q = &stackQueue{}
		d.queues[stackName] = q
	}

	if q.running {
		q.pending = appendCoalesced(q.pending, order)
		if shouldCancelInFlight(q.currentType, order) {
			q.cancel()
		}
		d.mu.Unlock()
		return
	}

	// First order for this stack: start a drain goroutine. The cancel
	// func is stored before releasing the lock so a concurrent Dispatch
	// observing running=true can always call it safely.
	runCtx, cancel := context.WithCancel(ctx)
	q.running = true
	q.cancel = cancel
	q.currentType = orderTypeOf(order)
	d.mu.Unlock()

	d.wp.Submit(func() { d.drain(ctx, stackName, runCtx, cancel, order) })
}

func (d *stackDispatcher) drain(parent context.Context, stackName string, firstCtx context.Context, firstCancel context.CancelFunc, first *generated.Order) {
	current := first
	currentCtx := firstCtx
	currentCancel := firstCancel

	for {
		d.handle(currentCtx, current)
		currentCancel()

		d.mu.Lock()
		q := d.queues[stackName]
		if len(q.pending) == 0 {
			delete(d.queues, stackName)
			d.mu.Unlock()
			return
		}
		current = q.pending[0]
		q.pending = q.pending[1:]
		currentCtx, currentCancel = context.WithCancel(parent)
		q.cancel = currentCancel
		q.currentType = orderTypeOf(current)
		d.mu.Unlock()
	}
}

type orderType uint8

const (
	orderUnknown orderType = iota
	orderExistingStack
	orderDeletedStack
	orderEnabledStack
	orderDisabledStack
)

func orderTypeOf(o *generated.Order) orderType {
	switch o.GetMessage().(type) {
	case *generated.Order_ExistingStack:
		return orderExistingStack
	case *generated.Order_DeletedStack:
		return orderDeletedStack
	case *generated.Order_EnabledStack:
		return orderEnabledStack
	case *generated.Order_DisabledStack:
		return orderDisabledStack
	}
	return orderUnknown
}

func stackNameFromOrder(o *generated.Order) string {
	switch m := o.GetMessage().(type) {
	case *generated.Order_ExistingStack:
		return m.ExistingStack.GetClusterName()
	case *generated.Order_DeletedStack:
		return m.DeletedStack.GetClusterName()
	case *generated.Order_EnabledStack:
		return m.EnabledStack.GetClusterName()
	case *generated.Order_DisabledStack:
		return m.DisabledStack.GetClusterName()
	}
	return ""
}

func appendCoalesced(pending []*generated.Order, order *generated.Order) []*generated.Order {
	if orderTypeOf(order) != orderExistingStack || len(pending) == 0 {
		return append(pending, order)
	}
	if orderTypeOf(pending[len(pending)-1]) != orderExistingStack {
		return append(pending, order)
	}
	pending[len(pending)-1] = order
	return pending
}

// shouldCancelInFlight reports whether an arriving order should cancel
// the in-flight one for the same stack. See stackDispatcher's doc
// comment for the rules.
func shouldCancelInFlight(current orderType, incoming *generated.Order) bool {
	if current == orderDeletedStack {
		return false
	}
	incomingType := orderTypeOf(incoming)
	if incomingType == orderDeletedStack {
		return true
	}
	if current == orderExistingStack && incomingType == orderExistingStack {
		return true
	}
	return false
}
