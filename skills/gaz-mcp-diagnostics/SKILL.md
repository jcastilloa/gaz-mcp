---
name: gaz-mcp-diagnostics
description: Diagnose running Go processes through gaz-mcp using pprof profiles, flame graphs, grouped goroutine stacks, expvar metrics, database/sql pool statistics, and evidence-backed heuristic suspicions. Use when investigating Go CPU, memory, allocation, contention, goroutine, file-descriptor, scheduler, cgroup, host-resource, or SQL-pool problems through a target exposing /debug/pprof and optionally /debug/vars.
---

# gaz-mcp Go diagnostics

Inspect a Go process through its absolute `target` URL. The target must expose `/debug/pprof`; metrics require `/debug/vars`.

## Choose the first tool

- Start broad with `diagnose(target)` when the cause is unknown. It returns raw observations and separate heuristic suspicions.
- Use `analyze_goroutines(target)` for blocking, leaks, connection waiters, or lock contention.
- Use `get_runtime_stats(target)` for a compact standard-runtime view.
- Use `get_db_pool_stats(target)` for instrumented `database/sql` pools.
- Use `get_expvars(target)` only when complete raw expvar data is necessary; it may be large or sensitive.
- Use profiling tools when a symptom needs code-level attribution.

## Interpret diagnose correctly

`diagnose` deliberately separates:

- `metrics.before` and `metrics.after`: raw runtime, process, host, cgroup, SQL-pool, and custom values.
- `goroutines`: grouped stack observations.
- `suspicions`: heuristic candidates with severity, confidence, evidence, reason, and a verification hint.

Treat metrics and stacks as observations. Treat every suspicion, including one marked `confirmed`, as a hypothesis supported by multiple signals rather than an established root cause. Check its evidence, consider workload and container context, and use the `verify` hint or a focused profile before concluding.

Counter-based heuristics depend on deltas between the two samples. Do not infer current pressure from an accumulated total alone. Distinguish host metrics from cgroup limits: inside a container, host capacity may not describe resources available to the process.

## Profile workflow

1. Call `capture_profile(target, type, seconds?, gc?)` and retain its `profile_id`.
2. Call `analyze_profile(profile_id, top_n?)` for raw values, percentages, and available rates.
3. Call `generate_flamegraph(profile_id)` when stack shape matters.
4. Call `diff_profiles(id_a, id_b, top_n?)` only for comparable profiles with the same sample type and unit.

Choose the profile type by symptom:

| Type | Use for |
|---|---|
| `cpu` | Sustained CPU or slow CPU-bound requests |
| `heap` | Retained Go heap memory |
| `allocs` | Allocation churn |
| `goroutine` | Goroutine growth or blocking |
| `block` | Time blocked on synchronization or channels |
| `mutex` | Lock contention |
| `threadcreate` | Unexpected operating-system thread creation |

CPU captures accept a duration from 1 to 60 seconds and default to 10. For heap, allocs, goroutine, block, mutex, and threadcreate, `seconds` requests an interval delta profile. `gc=true` is valid only for a heap snapshot without `seconds`. Use a short representative interval in production.

`analyze_profile` and `diff_profiles` expose `rates_available`. When true, use per-second values to compare different capture durations; retain raw deltas to understand total change. When false, compare only like-for-like snapshots and workloads.

Block and mutex profiles can be empty unless runtime sampling was enabled in the target. Absence of samples is not evidence that contention is absent.

## Verification patterns

- High RSS with stable Go memory: inspect cgroup headroom and validate possible native, cgo, or mmap growth rather than blaming the Go heap.
- SQL pool suspicion: correlate waiter stacks with pool saturation and wait deltas. With several pools, stacks cannot identify the responsible `*sql.DB` instance.
- Goroutine leak suspicion: compare the same stack fingerprint over time; one large stable worker group may be intentional.
- File-descriptor pressure: verify both open-FD ratio and trend; the instrumentation exposes counts, not paths.
- Scheduler or throttling pressure: correlate runnable goroutines, GOMAXPROCS, cgroup throttling deltas, and a CPU profile.

## Operational rules

- Diagnostic endpoints can expose internals. Use only authorised targets and do not reproduce sensitive expvars unnecessarily.
- Profile IDs reference files local to the gaz-mcp process and are temporary, not portable artifacts.
- Profile capture is active on the target; interval profiles sample the runtime and `gc=true` forces garbage collection. Avoid repeated or unnecessarily long captures.
- Report the raw evidence, the heuristic interpretation, and remaining uncertainty separately.
