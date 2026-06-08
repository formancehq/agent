# ADR 0001: Reconcile Kubernetes Status Snapshots With Membership

Status: Draft

Date: 2026-06-08

## Context

The agent currently synchronizes stack status from Kubernetes to Membership through Kubernetes informer events.

At a high level, the flow is:

1. Membership sends an `ExistingStack` order to the agent.
2. The agent creates or updates the Kubernetes `Stack` and module custom resources.
3. The operator reconciles those resources and writes `.status`.
4. The agent informers observe `Add`, `Update`, or `Delete` events.
5. The agent sends status updates back to Membership.

This makes the Membership view of a stack dependent on receiving the right informer transition at the right time. The agent does not currently have a separate reconciliation loop that periodically compares the current Kubernetes state with the Membership state or republishes current status snapshots.

## Incident Summary

While creating a sandbox stack, `fctl stack create` waited for several minutes even though the stack was available on the Kubernetes cluster after a short time.

Observed stack:

- Organization: `yghbiuocfzgk`
- Stack ID: `ybir`
- Cluster resource name: `yghbiuocfzgk-ybir`
- Region: `eu-sandbox`
- Agent setting: `RESYNC_PERIOD=30s`

Kubernetes state showed all resources ready:

```text
Stack    yghbiuocfzgk-ybir ready=True 2026-06-08T18:40:18Z
Auth     yghbiuocfzgk-ybir ready=True 2026-06-08T18:40:00Z
Gateway  yghbiuocfzgk-ybir ready=True 2026-06-08T18:40:18Z
Ledger   yghbiuocfzgk-ybir ready=True 2026-06-08T18:39:59Z
Payments yghbiuocfzgk-ybir ready=True 2026-06-08T18:39:59Z
Stargate yghbiuocfzgk-ybir ready=True 2026-06-08T18:39:23Z
```

The namespace deployments and pods were also available.

Membership state still showed:

```text
stack status=PROGRESSING synchronised=false reachable=true
auth     UNKNOWN
gateway  UNKNOWN
ledger   READY
payments READY
stargate READY
```

Agent logs showed status updates being detected for:

```text
18:39:51 stargates Update
18:40:25 stacks Update
18:41:36 ledgers Update
18:42:34 payments Update
```

No corresponding `auths Update` or `gateways Update` log was observed for `yghbiuocfzgk-ybir`, despite the Kubernetes `Auth` and `Gateway` custom resources having ready status.

## Current Behavior

Module informer updates only send a message when the Kubernetes `.status` changes between the old and new informer objects:

```go
if newStatus == nil || reflect.DeepEqual(oldStatus, newStatus) {
    return
}
```

Stack informer updates follow the same pattern, with an additional `spec.disabled` check:

```go
if newStatus == nil || (reflect.DeepEqual(oldStatus, newStatus) && oldDisabled == newDisabled) {
    return
}
```

`RESYNC_PERIOD` is correctly wired into the dynamic shared informer factory. However, informer resyncs still pass through the same update handlers. If Kubernetes already has a stable ready status, the resync can call `UpdateFunc` with equivalent old and new statuses. The handler then returns without sending anything to Membership.

As a result, reducing `RESYNC_PERIOD` does not guarantee Membership convergence after a missed, skipped, or lost status publication.

## Problem

The agent currently treats "the Kubernetes status did not change" as equivalent to "Membership already knows this status".

Those are not equivalent.

Membership can be stale even when Kubernetes is stable. This happens when:

- the informer observes an `Add` before `.status` exists and skips it;
- the status transition is missed, coalesced, or not delivered to the handler in the expected form;
- the agent sends the message but the gRPC stream is interrupted before Membership persists it;
- Membership rejects, drops, or fails to apply the message;
- the agent restarts while resources already have stable statuses;
- a resync replays an object whose status is unchanged relative to the informer cache.

In all of these cases, a purely edge-triggered design can leave Membership permanently behind Kubernetes.

## Why This Gets Worse With Many Stacks

On startup, informers list existing resources and trigger `AddFunc` for all objects currently in the cache. In an environment with many stacks, this can create a large burst of status messages and Kubernetes object handling.

This has two effects:

- initial agent startup becomes slower and noisier;
- fresh stack status updates compete with a large backlog of existing object events.

Even if this is not the only cause of the observed incident, it increases the probability and impact of delayed or missed publication.

## Decision Proposal

Keep informer events as a low-latency signal, but add an explicit status reconciliation path that publishes idempotent status snapshots from Kubernetes to Membership.

The agent should no longer rely only on status transitions. It should also periodically state: "this is the current Kubernetes status for this stack/module".

## Proposed Design

### 1. Status Snapshot Publisher

Add a component that periodically lists current Kubernetes `Stack` and module custom resources and sends their current statuses to Membership.

The publisher should:

- list resources by type (`stacks`, `auths`, `gateways`, `ledgers`, `payments`, `stargates`, and other supported modules);
- ignore objects without `.status`;
- send the current status even if it is identical to the previous informer cache value;
- include enough metadata to make the update idempotent and observable.

Suggested message metadata:

- resource kind;
- resource name / cluster stack name;
- resource version;
- status observed generation when available;
- status hash;
- publish reason: `event`, `resync`, `snapshot`, `startup`, `post-sync`.

### 2. Idempotent Membership Status Updates

Membership should accept duplicate status updates. Sending `Auth READY` multiple times must be safe.

Membership should update `lastStatusUpdate` only when appropriate. There are two reasonable options:

- update `lastStatusUpdate` on every accepted heartbeat-like snapshot;
- update `lastStatusUpdate` only when the semantic status changes, while separately tracking `lastStatusSeenAt`.

The second option provides better observability because it distinguishes status freshness from status changes.

### 3. Post-Stack-Sync Fast Path

After handling an `ExistingStack` order and creating/updating the Kubernetes resources, the agent should start a short targeted reconciliation for that stack.

For example:

- poll/list only that stack's `Stack` and module resources for up to 2 minutes;
- send each status as soon as it appears;
- stop once all expected modules have published a non-empty status or the timeout expires.

This directly improves `fctl stack create` latency without waiting for a global periodic snapshot.

### 4. Startup Behavior

Avoid an uncontrolled startup flood where all existing resources generate immediate writes with no prioritization.

Potential improvements:

- rate-limit snapshot publishing;
- prioritize recently created or non-synchronized stacks if Membership provides that information;
- deduplicate by `(kind, resource name, resourceVersion, status hash)`;
- expose queue depth and publish latency metrics.

### 5. Observability

The current logs do not clearly show skipped events. Add structured logs and metrics around status handling.

For informer handlers:

- resource kind;
- resource name;
- event type;
- old resource version;
- new resource version;
- old status hash;
- new status hash;
- whether `.status` was missing;
- skip reason;
- send result.

For the Membership client:

- message type;
- resource kind/name;
- enqueue latency;
- send latency;
- send error;
- stream reconnect count;
- dropped messages, if any.

For the snapshot publisher:

- snapshot run duration;
- resources scanned;
- statuses published;
- statuses skipped due missing status;
- publish failures;
- queue depth.

## Alternatives Considered

### Only Lower `RESYNC_PERIOD`

Rejected.

Lowering the informer resync period can increase the frequency of update callbacks, but it does not guarantee publication because the existing handlers skip unchanged statuses. This was observed with `RESYNC_PERIOD=30s`.

### Remove the `DeepEqual` Guard

Partially useful, but incomplete.

Always sending on every informer update would make resyncs publish status, but it can create noisy duplicate traffic and still does not address startup ordering, stream acknowledgement, or targeted convergence after stack creation.

This could be acceptable only if paired with rate limiting and idempotency.

### Restart the Agent When Membership Is Stale

Rejected as a fix.

Restarting the agent may trigger `AddFunc` for existing resources and can temporarily repair stale Membership state. However, it creates a startup event flood and does not address the underlying convergence problem.

## Risks

Publishing status snapshots can increase write volume to Membership.

Mitigations:

- deduplicate using resource version and status hash;
- rate-limit global snapshots;
- prioritize targeted snapshots after stack sync;
- make Membership updates idempotent;
- add metrics before enabling aggressive intervals.

## Rollout Plan

1. Add observability around informer skips and Membership sends.
2. Add idempotent status handling in Membership if needed.
3. Add a targeted post-sync status publisher for newly synced stacks.
4. Add a periodic global snapshot publisher with conservative defaults.
5. Enable in sandbox first.
6. Compare stack create wait time before and after rollout.
7. Tune interval, rate limits, and deduplication.

## Validation Plan

Create an integration test that reproduces stale Membership state:

1. Create a stack and module CR with no `.status`.
2. Let the informer observe the `Add`.
3. Set `.status.ready=true`.
4. Simulate a missed or skipped update, or start the snapshot publisher after the status is already stable.
5. Assert Membership receives the current status anyway.

Add tests for:

- duplicate status updates are safe;
- resync/snapshot publishes stable statuses;
- missing statuses are skipped with a visible reason;
- post-sync targeted reconciliation sends all expected module statuses;
- stream reconnect does not permanently lose queued status updates without visibility.

## Open Questions

- Should Membership expose stack/module synchronization state to the agent so snapshots can prioritize stale stacks?
- Should status messages include a monotonic sequence or only Kubernetes `resourceVersion`?
- Should `lastStatusUpdate` mean "semantic status changed" or "status was last observed by Membership"?
- What is an acceptable snapshot interval and write rate for production regions?
- Should the agent keep a local persisted outbox for status updates, or is periodic snapshot reconciliation sufficient?

## Expected Outcome

The Membership view of stack status should converge to the Kubernetes view even if individual informer events are skipped, coalesced, delayed, or lost.

`fctl stack create` should stop waiting on stale `UNKNOWN` module statuses when the corresponding Kubernetes resources are already ready.
