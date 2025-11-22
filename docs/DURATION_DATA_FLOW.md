# Duration Module - Data Flow Diagrams

This document illustrates the data transformations and flow through the Duration module.

## Data Structure Evolution

### From Heartbeat to Summary

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Data Flow Overview                           │
└─────────────────────────────────────────────────────────────────────┘

Heartbeat                Duration               Summary
(Raw Input)             (Aggregated)           (Final Output)

┌──────────┐           ┌──────────┐           ┌───────────┐
│ Time: t1 │           │ Time: t1 │           │ Projects: │
│ Project  │           │ Duration │           │  - wakapi │
│ Language │──group──▶ │ Project  │──sum──▶   │    8 min  │
│ Editor   │   by      │ Language │   by      │ Languages:│
│ ...      │ timeout   │ Editor   │ entity    │  - Go     │
└──────────┘  & attrs  │ ...      │  type     │    8 min  │
                        │ NumBeats │           │ Editors:  │
┌──────────┐           └──────────┘           │  - VSCode │
│ Time: t2 │                                   │    12 min │
│ Project  │           ┌──────────┐           └───────────┘
│ Language │           │ Time: t8 │
│ Editor   │           │ Duration │
│ ...      │           │ Project  │
└──────────┘           │ Language │
     ...               │ Editor   │
                       │ ...      │
┌──────────┐           │ NumBeats │
│ Time: t8 │           └──────────┘
│ Project  │
│ Language │
│ Editor   │
│ ...      │
└──────────┘
```

## Detailed Data Transformations

### 1. Heartbeat Structure

```
Heartbeat (models/heartbeat.go)
├── ID: 12345678
├── UserID: "john_doe"
├── Time: 2025-01-15T10:00:00Z
├── Entity: "/home/john/wakapi/service.go"
├── Type: "file"
├── Category: "coding"
├── Project: "wakapi"
├── Language: "Go"
├── Editor: "VS Code"
├── OperatingSystem: "Linux"
├── Machine: "john-laptop"
├── Branch: "main"
├── IsWrite: true
├── Hash: "a1b2c3d4e5f6..."
└── CreatedAt: 2025-01-15T10:00:05Z

Properties:
- Immutable once created
- Stored indefinitely in database
- Unique hash prevents duplicates
- All attributes are strings or booleans
```

### 2. Duration Structure

```
Duration (models/duration.go)
├── ID: 789
├── UserID: "john_doe"
├── Time: 2025-01-15T10:00:00Z          ← Start time (first heartbeat)
├── Duration: 480 seconds (8 minutes)   ← Computed length
├── Project: "wakapi"                   ← From heartbeats
├── Language: "Go"                      ← From heartbeats
├── Editor: "VS Code"                   ← From heartbeats
├── OperatingSystem: "Linux"            ← From heartbeats
├── Machine: "john-laptop"              ← From heartbeats
├── Category: "coding"                  ← From heartbeats
├── Branch: "main"                      ← From heartbeats
├── Entity: "/home/john/wakapi/service.go"  ← Most prominent file
├── NumHeartbeats: 5                    ← Count of aggregated heartbeats
├── GroupHash: "f6e5d4c3b2a1"          ← Hash of attributes (for grouping)
└── Timeout: 600 seconds                ← Heartbeat timeout used

Properties:
- Compressed representation of multiple heartbeats
- Stores aggregated time span
- Much smaller storage than raw heartbeats
- Can be recomputed from heartbeats
```

### 3. Summary Structure

```
Summary (models/summary.go)
├── ID: 456
├── UserID: "john_doe"
├── FromTime: 2025-01-15T00:00:00Z
├── ToTime: 2025-01-15T23:59:59Z
├── Projects: [
│   ├── {Key: "wakapi", Total: 14400, Type: SummaryProject}      (4 hours)
│   └── {Key: "frontend", Total: 7200, Type: SummaryProject}     (2 hours)
│   ]
├── Languages: [
│   ├── {Key: "Go", Total: 14400, Type: SummaryLanguage}         (4 hours)
│   └── {Key: "TypeScript", Total: 7200, Type: SummaryLanguage}  (2 hours)
│   ]
├── Editors: [
│   └── {Key: "VS Code", Total: 21600, Type: SummaryEditor}      (6 hours)
│   ]
├── OperatingSystems: [
│   └── {Key: "Linux", Total: 21600, Type: SummaryOS}            (6 hours)
│   ]
├── Machines: [
│   └── {Key: "john-laptop", Total: 21600, Type: SummaryMachine} (6 hours)
│   ]
├── Categories: [
│   └── {Key: "coding", Total: 21600, Type: SummaryCategory}     (6 hours)
│   ]
├── NumHeartbeats: 180
└── Total: 21600 seconds (6 hours)

Properties:
- Aggregated by entity types
- All times in seconds
- Sorted by total descending
- Can include branches & entities (optional)
```

## Transformation Pipeline

### Pipeline 1: Heartbeat → Duration (Grouping)

```
Input: Stream of Heartbeats
Output: Array of Durations

┌─────────────────────────────────────────────────────────┐
│                 Grouping Algorithm                      │
└─────────────────────────────────────────────────────────┘

Step 1: Create initial duration from first heartbeat
┌──────────────────┐
│ H1: t=00:00      │ → Create D1 with Duration=0
│   project=wakapi │    GroupHash = Hash(wakapi, Go, VSCode, ...)
└──────────────────┘

Step 2: Process subsequent heartbeats within timeout
┌──────────────────┐
│ H2: t=00:02      │ → Same hash? YES
│   project=wakapi │    diff = 2 min - 0 = 2 min
└──────────────────┘    D1.Duration += 2 min
                        D1.NumHeartbeats++

┌──────────────────┐
│ H3: t=00:04      │ → Same hash? YES
│   project=wakapi │    diff = 4 min - 2 min = 2 min
└──────────────────┘    D1.Duration += 2 min
                        D1.NumHeartbeats++

Step 3: Handle different attributes (new hash)
┌──────────────────┐
│ H4: t=00:15      │ → Same hash? NO (different project)
│ project=frontend │    Create D2 with Duration=0
└──────────────────┘    GroupHash = Hash(frontend, TS, VSCode, ...)

Step 4: Handle timeout exceeded
┌──────────────────┐
│ H5: t=01:30      │ → diff = 1:30 - 0:17 = 73 min
│ project=frontend │    diff > timeout (10 min)
└──────────────────┘    Create D3 with Duration=0

Result:
├── D1: t=00:00, duration=8min, project=wakapi, heartbeats=5
├── D2: t=00:15, duration=4min, project=frontend, heartbeats=3
└── D3: t=01:30, duration=0min, project=frontend, heartbeats=1
```

### Pipeline 2: Duration → Summary (Aggregation)

```
Input: Array of Durations
Output: Summary with items by entity type

┌─────────────────────────────────────────────────────────┐
│              Aggregation by Entity Type                 │
└─────────────────────────────────────────────────────────┘

Durations:
┌─────────────────────────────────────────┐
│ D1: wakapi, Go, 8min                    │
│ D2: frontend, TypeScript, 4min          │
│ D3: wakapi, Go, 6min                    │
└─────────────────────────────────────────┘

Parallel Aggregation:

By Project:                    By Language:
mapping = {}                   mapping = {}
for D in durations:            for D in durations:
  mapping[D.Project] += D.dur    mapping[D.Language] += D.dur

Result:                        Result:
wakapi: 8 + 6 = 14min         Go: 8 + 6 = 14min
frontend: 4min                 TypeScript: 4min

By Editor:                     By OS:
mapping = {}                   mapping = {}
for D in durations:            for D in durations:
  mapping[D.Editor] += D.dur     mapping[D.OS] += D.dur

Result:                        Result:
VS Code: 18min                 Linux: 18min

Final Summary:
┌──────────────────────────────────┐
│ Projects: [wakapi: 14, frontend: 4] │
│ Languages: [Go: 14, TypeScript: 4]  │
│ Editors: [VS Code: 18]              │
│ OS: [Linux: 18]                     │
│ Total: 18 minutes                   │
└──────────────────────────────────┘
```

## Caching Flow

### Cached vs Live Duration Computation

```
┌─────────────────────────────────────────────────────────┐
│         Duration Retrieval Strategy                     │
└─────────────────────────────────────────────────────────┘

Request: Get durations for [2025-01-15 00:00, 2025-01-15 23:59]

Step 1: Check if can use cache
┌────────────────────────────────┐
│ skipCache = false?             │ YES
│ customTimeout = user.timeout?  │ YES → Try cache
└────────────────────────────────┘

Step 2: Query cached durations
┌────────────────────────────────┐
│ SELECT * FROM durations        │
│ WHERE user_id = 'john'         │
│   AND time >= '2025-01-15 00:00' │
│   AND time <= '2025-01-15 23:59' │
└────────────────────────────────┘
       ↓
   [D1, D2, D3]  (cached until 2025-01-15 20:00)

Step 3: Check if cache is complete
┌────────────────────────────────┐
│ cached.Last().TimeEnd() < to?  │ YES → Gap exists
└────────────────────────────────┘
       ↓
   Gap: [2025-01-15 20:00, 2025-01-15 23:59]

Step 4: Fill missing gap (live computation)
┌────────────────────────────────┐
│ SELECT * FROM heartbeats       │
│ WHERE user_id = 'john'         │
│   AND time >= '2025-01-15 20:00' │
│   AND time <= '2025-01-15 23:59' │
└────────────────────────────────┘
       ↓
   Heartbeats → Compute → [D4, D5]

Step 5: Merge cached + live
┌────────────────────────────────┐
│ merge([D1,D2,D3], [D4,D5])    │
│                                │
│ Check: D4.Time > D3.TimeEnd?   │ YES → No overlap
│                                │
│ Can merge D3 and D4?           │
│   diff = D4.Time - D3.TimeEnd  │
│   diff < timeout?              │ Check
│   D3.hash == D4.hash?          │ Check
└────────────────────────────────┘
       ↓
   Final: [D1, D2, D3-merged-D4, D5]
```

## Storage Efficiency

### Compression Ratio

```
Example: 1 hour of coding with heartbeat every 2 minutes

Raw Heartbeats:
- 30 heartbeats per hour
- ~500 bytes per heartbeat
- Total: 15,000 bytes

Durations:
- 1-5 durations per hour (depending on project switches)
- ~300 bytes per duration
- Total: ~1,500 bytes

Compression: ~10x reduction

Summary (per day):
- 1 summary record
- ~2,000 bytes (including all entity types)
- Total: 2,000 bytes

Overall: Durations provide middle ground between raw data and summaries
```

## Data Retention

```
┌─────────────────────────────────────────────────────────┐
│                  Data Lifecycle                         │
└─────────────────────────────────────────────────────────┘

Heartbeats (Permanent):
├── Stored forever
├── Used for: Live computation, regeneration, debugging
└── Largest storage footprint

Durations (Ephemeral Cache):
├── Regenerated every 12 hours
├── Can be deleted and recomputed from heartbeats
├── Used for: Fast summary generation
└── Medium storage footprint

Summaries (Optional Persistence):
├── Can be persisted for faster access
├── Pre-computed aggregations
├── Used for: Historical data, reports
└── Smallest storage footprint

Database Tables:
┌──────────────┬────────────┬──────────────┐
│ heartbeats   │ durations  │ summaries    │
├──────────────┼────────────┼──────────────┤
│ 1M records   │ 100K recs  │ 365 recs     │
│ (1 year)     │ (cache)    │ (1 year)     │
└──────────────┴────────────┴──────────────┘
```

## Performance Characteristics

### Time Complexity

```
Operation                     Complexity    Notes
────────────────────────────────────────────────────────
Insert Heartbeat              O(1)          Direct DB insert
Compute Durations (live)      O(n)          n = # of heartbeats
Query Cached Durations        O(log n)      Indexed by time + user
Merge Durations               O(m + k)      m = cached, k = missing
Aggregate to Summary          O(d × t)      d = # durations, t = # types
                                            Parallelized

Full Regeneration             O(n)          n = all heartbeats for user
                                            Runs in background
```

### Space Complexity

```
Storage                       Space         Notes
────────────────────────────────────────────────────────
Heartbeat                     ~500 bytes    All attributes + metadata
Duration                      ~300 bytes    Aggregated representation
Summary                       ~2 KB         All entity type aggregations

In-Memory Processing:
- Heartbeat streaming         O(1)          Processed one at a time
- Duration mapping            O(d)          d = # of distinct durations
- Summary aggregation         O(d × t)      Temporary mappings
```

## Key Takeaways

1. **Heartbeats** are the source of truth - immutable, permanent
2. **Durations** are computed aggregations - can be cached for performance
3. **Summaries** are final aggregations - optimized for display
4. **Caching** is crucial for performance - recomputation is expensive
5. **Merging** allows incremental updates - no need to recompute everything
6. **Parallel aggregation** speeds up summary generation
7. **Streaming** keeps memory usage low for large datasets
