# Visual Architecture Overview

This document provides visual diagrams for understanding the Duration computation module at a glance.

## System Architecture

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                     WAKAPI DURATION SYSTEM                      ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

┌─────────────────┐
│  Code Editor    │  Sends heartbeat every ~2 minutes
│  (VS Code,      │  while actively coding
│   IntelliJ)     │
└────────┬────────┘
         │ POST /api/heartbeat
         │ {time, project, language, editor, ...}
         ↓
┌─────────────────────────────────────────────────────────────────┐
│                    HEARTBEAT PROCESSING                         │
├─────────────────────────────────────────────────────────────────┤
│  1. Validate & Sanitize                                         │
│  2. Store in Database (heartbeats table)                        │
│  3. Publish HeartbeatCreate event                               │
└────────┬────────────────────────────────────────────────────────┘
         │
         ↓
┌─────────────────────────────────────────────────────────────────┐
│                    EVENT BUS / JOB QUEUE                        │
├─────────────────────────────────────────────────────────────────┤
│  • Check if regeneration needed (12h since last)                │
│  • Queue background job to regenerate durations                 │
└────────┬────────────────────────────────────────────────────────┘
         │
         ↓
┌─────────────────────────────────────────────────────────────────┐
│               DURATION COMPUTATION (Background)                 │
├─────────────────────────────────────────────────────────────────┤
│  Input:  Stream of Heartbeats                                   │
│          [H1, H2, H3, ..., Hn]                                  │
│                                                                 │
│  Process:                                                       │
│  ┌─────────────────────────────────────────┐                   │
│  │ FOR each heartbeat H:                   │                   │
│  │   IF time_gap < timeout AND            │                   │
│  │      same_attributes:                   │                   │
│  │     ➜ Add to current duration          │                   │
│  │   ELSE:                                 │                   │
│  │     ➜ Start new duration               │                   │
│  └─────────────────────────────────────────┘                   │
│                                                                 │
│  Output: Array of Durations                                     │
│          [D1, D2, D3, ..., Dm]  (m << n)                       │
│                                                                 │
│  Store:  INSERT INTO durations table (cache)                    │
└─────────────────────────────────────────────────────────────────┘
         ▲
         │
         │ On API request
         │
┌────────┴────────────────────────────────────────────────────────┐
│               SUMMARY GENERATION (On Demand)                    │
├─────────────────────────────────────────────────────────────────┤
│  GET /api/summary?from=...&to=...                               │
│                                                                 │
│  Step 1: Get Durations                                          │
│  ┌──────────────────────────────────────┐                      │
│  │ • Try cache first (fast path)        │                      │
│  │ • Fill gaps if needed (live compute) │                      │
│  │ • Merge cached + live durations      │                      │
│  └──────────────────────────────────────┘                      │
│                                                                 │
│  Step 2: Aggregate by Entity Type (Parallel)                    │
│  ┌──────────────┬──────────────┬──────────────┐               │
│  │   Projects   │   Languages  │   Editors    │               │
│  │ wakapi: 14m  │  Go: 14m     │ VSCode: 18m  │               │
│  │ frontend: 4m │  TS: 4m      │              │               │
│  └──────────────┴──────────────┴──────────────┘               │
│                                                                 │
│  Step 3: Return JSON Summary                                    │
│  {                                                              │
│    "projects": [{key: "wakapi", total: 840}, ...],             │
│    "languages": [{key: "Go", total: 840}, ...],                │
│    "total": 1080,                                               │
│    "num_heartbeats": 36                                         │
│  }                                                              │
└─────────────────────────────────────────────────────────────────┘
```

## Data Flow Diagram

```
HEARTBEAT                 DURATION                  SUMMARY
(Source)                  (Cache)                   (Output)

┌─────────┐              ┌─────────┐              ┌──────────┐
│ t: 00:00│              │ t: 00:00│              │ Projects:│
│ proj: A │              │ dur: 8m │              │  A: 8min │
│ lang: Go│──┐           │ proj: A │──┐           │  B: 4min │
└─────────┘  │           │ lang: Go│  │           │          │
             │           │ beats: 5│  │           │Languages:│
┌─────────┐  │           └─────────┘  │           │  Go: 8m  │
│ t: 00:02│  │                         │           │  TS: 4m  │
│ proj: A │  ├─Group────▶              ├─Aggregate▶│          │
│ lang: Go│  │  by       ┌─────────┐  │   by      │ Editors: │
└─────────┘  │ timeout   │ t: 00:15│  │  entity   │ VSCode:  │
             │   &       │ dur: 4m │  │   type    │  12min   │
┌─────────┐  │ attributes│ proj: B │  │           │          │
│ t: 00:15│  │           │ lang: TS│  │           │ Total:   │
│ proj: B │  │           │ beats: 3│  │           │  12min   │
│ lang: TS│──┘           └─────────┘  │           │          │
└─────────┘                            │           │ Beats: 8 │
    ...                                │           └──────────┘
                                       │
Storage: ~500 bytes/hb   ~300 bytes/d │  ~2 KB/summary
Total:   ~100 KB/day     ~10 KB/day   │  2 KB/day
         (permanent)     (cache)       │  (optional)
```

## Cache Strategy Visualization

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃          HYBRID CACHING STRATEGY                     ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

Request: Get summary for [Jan 1, Jan 15]

┌────────────────────────────────────────────────────┐
│ Check Cache                                        │
└────────────────┬───────────────────────────────────┘
                 │
                 ↓
         ┌───────────────┐
         │ Cache Status? │
         └───┬───────┬───┘
             │       │
      ┌──────┘       └──────┐
      │                     │
   [MISS]                [HIT]
      │                     │
      ↓                     ↓
┌──────────────┐   ┌────────────────┐
│ Compute Live │   │ Check Coverage │
│              │   │                │
│ From: Jan 1  │   │ Cached: Jan 1  │
│ To:   Jan 15 │   │       → Jan 10 │
│              │   │ Missing: Jan 10│
│ (slow path)  │   │        → Jan 15│
└──────────────┘   └────────┬───────┘
                            │
                            ↓
                   ┌────────────────┐
                   │ Fill Gap       │
                   │                │
                   │ Compute Live:  │
                   │ From: Jan 10   │
                   │ To:   Jan 15   │
                   │                │
                   │ (hybrid path)  │
                   └────────┬───────┘
                            │
                            ↓
                   ┌────────────────┐
                   │ Merge          │
                   │                │
                   │ cached +       │
                   │ missing →      │
                   │ complete       │
                   └────────┬───────┘
                            │
         ┌──────────────────┴──────────────────┐
         │                                     │
         ↓                                     ↓
┌────────────────┐                    ┌────────────────┐
│ Apply Filters  │                    │ Apply Filters  │
└────────┬───────┘                    └────────┬───────┘
         │                                     │
         └──────────────┬──────────────────────┘
                        │
                        ↓
                ┌───────────────┐
                │ Return Result │
                └───────────────┘
```

## Performance Comparison

```
┌──────────────────────────────────────────────────────┐
│         COMPUTATION TIME COMPARISON                  │
└──────────────────────────────────────────────────────┘

Scenario: Generate summary for 1 day of coding
          (Assume 8 hours, heartbeat every 2 min = 240 heartbeats)

┌─────────────────────┬────────────┬──────────────────┐
│ Method              │ Time       │ Notes            │
├─────────────────────┼────────────┼──────────────────┤
│ From Heartbeats     │ ~500ms     │ • Slow           │
│ (No Cache)          │            │ • Always fresh   │
│                     │            │ • High DB load   │
├─────────────────────┼────────────┼──────────────────┤
│ From Cache          │ ~50ms      │ • Fast!          │
│ (Complete)          │            │ • May be stale   │
│                     │            │ • Low DB load    │
├─────────────────────┼────────────┼──────────────────┤
│ Hybrid              │ ~150ms     │ • Good balance   │
│ (Cache + Fill Gap)  │            │ • Up-to-date     │
│                     │            │ • Optimal        │
└─────────────────────┴────────────┴──────────────────┘

Performance Gain: 3-10x faster with cache
```

## Storage Efficiency

```
┌──────────────────────────────────────────────────────┐
│         STORAGE FOOTPRINT COMPARISON                 │
└──────────────────────────────────────────────────────┘

For 1 Year of Coding (8 hrs/day, 250 days)

┌─────────────────────┬──────────────┬──────────────┐
│ Data Type           │ Records      │ Storage      │
├─────────────────────┼──────────────┼──────────────┤
│ Heartbeats          │ ~960,000     │ ~480 MB      │
│ (permanent)         │ (240/day)    │ (source)     │
├─────────────────────┼──────────────┼──────────────┤
│ Durations           │ ~100,000     │ ~30 MB       │
│ (cache)             │ (varies)     │ (16x less)   │
├─────────────────────┼──────────────┼──────────────┤
│ Summaries           │ ~365         │ ~730 KB      │
│ (optional)          │ (1/day)      │ (660x less)  │
└─────────────────────┴──────────────┴──────────────┘

Conclusion: Durations provide excellent middle ground
            between raw data and aggregated summaries
```

## Key Takeaways

### ✅ What Durations Are Good For
- **Fast queries**: Pre-aggregated data, ~10x faster
- **Storage efficiency**: ~10x compression vs heartbeats
- **Incremental updates**: Only compute what's missing
- **Flexibility**: Can recompute from heartbeats anytime

### ⚠️ What to Watch Out For
- **Cache staleness**: Max 12 hours old by default
- **Custom timeouts**: Always bypass cache (slower)
- **File-level details**: Approximated, not precise
- **Memory usage**: Streaming helps, but large ranges can be heavy

### 🎯 Best Practices
1. Use cache whenever possible (default behavior)
2. Let background jobs handle regeneration
3. Don't request unnecessarily large time ranges
4. Use filters to reduce result size
5. Cache summaries at application level for frequently accessed data

### 🔧 For Developers
- **Modify timeout logic**: See `services/duration.go` line 226-233
- **Change aggregation**: See `services/summary.go` line 263-284
- **Adjust cache strategy**: See `services/duration.go` line 68-112
- **Optimize storage**: See `models/duration.go` for structure

---

See [DURATION_COMPUTATION.md](./DURATION_COMPUTATION.md) for detailed documentation.
