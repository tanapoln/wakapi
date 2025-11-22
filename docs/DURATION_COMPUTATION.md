# Duration Computation Module

## Overview

The Duration Computation module is a core component of Wakapi that transforms raw heartbeat data from code editors into meaningful time durations for generating activity summaries. This document explains how durations are computed, cached, and aggregated.

## Key Concepts

### What is a Heartbeat?

A **Heartbeat** is a single data point sent by a code editor plugin (e.g., WakaTime plugin) when you are actively coding. Each heartbeat contains:
- **Timestamp** - When the activity occurred
- **Entity** - The file being edited
- **Project** - The project name
- **Language** - Programming language
- **Editor** - Code editor being used
- **Operating System** - OS information
- **Machine** - Computer identifier
- **Branch** - Git branch (if applicable)
- **Category** - Type of activity (coding, browsing, etc.)

Heartbeats are sent approximately every 2 minutes while you're actively coding.

### What is a Duration?

A **Duration** is an aggregated time span computed from one or more consecutive heartbeats that occurred within a specific timeout period. Durations group related heartbeats together to calculate how long you spent on a particular activity.

Key properties of a Duration:
- **Time** - Start time (time of first heartbeat in the group)
- **Duration** - Length of time spent on the activity
- **NumHeartbeats** - Number of heartbeats aggregated
- **GroupHash** - Hash to identify similar activities
- **Timeout** - The maximum gap allowed between heartbeats (default: 10 minutes)
- **Entity attributes** - Project, Language, Editor, OS, Machine, Branch, Category

### Heartbeat Timeout

The **Heartbeat Timeout** (default: 10 minutes = 600 seconds) is the maximum time gap allowed between two consecutive heartbeats to be considered part of the same coding session. If heartbeats are more than this timeout apart, they are treated as separate durations.

## Architecture

```
┌────────────────┐
│  Code Editor   │ 
│   (VS Code,    │
│   IntelliJ,    │
│   etc.)        │
└───────┬────────┘
        │ Heartbeat sent every ~2 minutes
        ↓
┌───────────────────────────────────────────────┐
│            Wakapi Backend                     │
│                                               │
│  ┌─────────────────────────────────────┐     │
│  │  1. Heartbeat Collection             │     │
│  │     - Receive heartbeat              │     │
│  │     - Validate & sanitize            │     │
│  │     - Store in database              │     │
│  │     - Trigger regeneration event     │     │
│  └──────────────┬──────────────────────┘     │
│                 │                             │
│                 ↓                             │
│  ┌─────────────────────────────────────┐     │
│  │  2. Duration Generation (Live)       │     │
│  │     - Fetch heartbeats in time range │     │
│  │     - Group by timeout & attributes  │     │
│  │     - Calculate duration for each    │     │
│  │       group                          │     │
│  └──────────────┬──────────────────────┘     │
│                 │                             │
│                 ↓                             │
│  ┌─────────────────────────────────────┐     │
│  │  3. Duration Caching                 │     │
│  │     - Store computed durations       │     │
│  │     - Use cache for subsequent       │     │
│  │       queries                        │     │
│  │     - Fill missing gaps live         │     │
│  └──────────────┬──────────────────────┘     │
│                 │                             │
│                 ↓                             │
│  ┌─────────────────────────────────────┐     │
│  │  4. Summary Aggregation              │     │
│  │     - Group durations by entity type │     │
│  │       (project, language, editor,    │     │
│  │       OS, machine, branch, etc.)     │     │
│  │     - Sum total time for each        │     │
│  │     - Apply filters & aliases        │     │
│  └──────────────┬──────────────────────┘     │
│                 │                             │
│                 ↓                             │
│  ┌─────────────────────────────────────┐     │
│  │  5. Summary Presentation             │     │
│  │     - Format for UI/API              │     │
│  │     - Sort by time spent             │     │
│  │     - Apply user preferences         │     │
│  └─────────────────────────────────────┘     │
│                                               │
└───────────────────────────────────────────────┘
```

## Duration Computation Algorithm

### Step 1: Fetch Heartbeats

When a duration request is made for a time range `[from, to]`, the system:
1. Retrieves all heartbeats for the user within that time range
2. Sorts heartbeats by timestamp in ascending order
3. Streams them for processing

### Step 2: Group Heartbeats into Durations

The algorithm processes heartbeats sequentially and groups them based on:

```
For each heartbeat H:
  1. Create a Duration object D from H with:
     - Time = H.Time
     - Duration = 0
     - GroupHash = Hash(Project, Language, Editor, OS, Machine, Category, Branch)
     - NumHeartbeats = 1
  
  2. If this is the first heartbeat:
     - Add D to the mapping
     - Set D as "latest"
     - Continue
  
  3. Calculate time difference from previous heartbeat:
     diff = H.Time - (latest.Time + latest.Duration)
  
  4. Check if heartbeats should be grouped together:
     shouldGroup = (
       diff < HeartbeatTimeout AND
       D.GroupHash == latest.GroupHash AND
       sameDay(H.Time, latest.Time)
     )
  
  5. If shouldGroup:
     - Increment latest.Duration by diff
     - Increment latest.NumHeartbeats
     - Update latest.Entity to most prominent file
  
  6. Else (start new group):
     - Add D to mapping as new duration
     - Set D as "latest"
```

### Step 3: Duration Merging

When combining cached durations with newly computed ones:

```
Given: 
  - cached: existing durations from cache [D1, D2, ..., Dn]
  - missing: newly computed durations [Dn+1, Dn+2, ..., Dm]

1. Check overlap:
   - If Dn+1.Time < Dn.TimeEnd: ERROR (overlap detected)

2. Check if Dn and Dn+1 can be merged:
   middleDiff = Dn+1.Time - Dn.TimeEnd
   
   If middleDiff < HeartbeatTimeout:
     If Dn.GroupHash == Dn+1.GroupHash:
       - Merge into single duration
       - Duration = Dn.Duration + middleDiff + Dn+1.Duration
       - NumHeartbeats = Dn.NumHeartbeats + Dn+1.NumHeartbeats
     Else:
       - Extend Dn by middleDiff
       - Keep Dn+1 separate
   Else:
     - Keep both as separate durations

3. Append remaining missing durations
```

## Caching Strategy

### Cache Invalidation

Durations are regenerated:
- **Periodically**: Every 12 hours for each user after their first heartbeat
- **On-demand**: When explicitly requested with `skipCache=true`
- **Automatically**: When a custom timeout is different from user's preference

### Cache Structure

```
Database Table: durations
├── id: Primary key
├── user_id: User identifier (indexed)
├── time: Start time of duration (indexed)
├── duration: Length of time span
├── num_heartbeats: Count of aggregated heartbeats
├── group_hash: Hash for identifying similar activities
├── timeout: Heartbeat timeout used for computation
├── project, language, editor, os, machine, branch, category, entity
```

### Hybrid Approach

When fetching durations for `[from, to]`:

1. **Try cache first**: Query cached durations from database
2. **Identify gaps**: Check if cached data covers entire range
3. **Fill missing**: 
   - If gap at end: Compute live from `cached.Last().TimeEnd` to `to`
   - If no cache: Compute entire range live
4. **Merge**: Combine cached and live durations intelligently

## Summary Generation

### Aggregation by Entity Type

Once durations are obtained, they are aggregated by different entity types:

```go
// Parallel aggregation
for each summaryType in [Project, Language, Editor, OS, Machine, Branch, Entity, Category]:
  mapping = {}
  for each duration in durations:
    key = duration.GetKey(summaryType)
    mapping[key] += duration.Duration
  
  // Convert to SummaryItems
  items = []
  for key, totalTime in mapping:
    items.append(SummaryItem{Key: key, Total: totalTime})
  
  // Sort by total time descending
  sort(items, by: Total DESC)
```

### Applying Filters

Filters are applied **after** duration computation to avoid false positives:

```
Example: User worked on Project A and Project B in parallel
- If we filtered heartbeats before computing durations, gaps would be incorrectly calculated
- Instead, we compute all durations first, then filter out non-matching ones
```

### Result Structure

Final Summary contains:
- **FromTime, ToTime**: Time range
- **Projects**: List of projects with time spent
- **Languages**: List of languages with time spent
- **Editors**: List of editors with time spent
- **OperatingSystems**: List of OS with time spent
- **Machines**: List of machines with time spent
- **Branches**: List of branches with time spent (if details requested)
- **Entities**: List of files with time spent (if details requested)
- **Categories**: List of categories with time spent
- **NumHeartbeats**: Total heartbeats processed

Each item includes:
- **Key**: Name (e.g., "Python", "VS Code")
- **Total**: Time spent in seconds
- **Type**: Entity type identifier

## Important Notes

### Why Group by Hash?

The `GroupHash` is computed from entity attributes (excluding Entity/file name by default). This allows:
- **Efficient grouping**: O(1) lookup to find matching duration
- **Compression**: Reduces storage compared to raw heartbeats
- **Flexibility**: Can be configured to include/exclude entities

### Why Exclude Entity from GroupHash?

By default, individual file names are excluded from the hash to:
- **Reduce cardinality**: Avoid creating separate durations for every file
- **Better compression**: One duration for entire coding session on a project
- **Trade-off**: File-level details are approximated (most prominent file is kept)

### Multi-day Handling

Durations never span across day boundaries:
- Even if heartbeats are within timeout
- Ensures consistency with daily summaries
- Prevents edge cases in aggregation

### Timeout Customization

Users can customize their heartbeat timeout:
- **Default**: 10 minutes (600 seconds)
- **Effect**: Larger timeout = longer durations, fewer gaps
- **Live computation**: Custom timeouts always bypass cache

## Example Walkthrough

### Scenario

User codes for 30 minutes with these heartbeats:

```
00:00:00 - Project: wakapi, Language: Go, Editor: VS Code
00:02:00 - Project: wakapi, Language: Go, Editor: VS Code
00:04:00 - Project: wakapi, Language: Go, Editor: VS Code
00:06:00 - Project: wakapi, Language: Go, Editor: VS Code
00:08:00 - Project: wakapi, Language: Go, Editor: VS Code
00:15:00 - Project: frontend, Language: TypeScript, Editor: VS Code
00:17:00 - Project: frontend, Language: TypeScript, Editor: VS Code
00:19:00 - Project: frontend, Language: TypeScript, Editor: VS Code
```

### Duration Computation (Timeout = 10 minutes)

**Duration 1:**
- Time: 00:00:00
- Duration: 8 minutes (00:00 to 00:08, calculated from gaps)
- Project: wakapi, Language: Go, Editor: VS Code
- NumHeartbeats: 5

**Gap:** 7 minutes (00:08 to 00:15) - Within timeout, but different project

**Duration 2:**
- Time: 00:15:00
- Duration: 4 minutes (00:15 to 00:19)
- Project: frontend, Language: TypeScript, Editor: VS Code
- NumHeartbeats: 3

### Summary Generation

From these 2 durations:

**By Project:**
- wakapi: 8 minutes
- frontend: 4 minutes

**By Language:**
- Go: 8 minutes
- TypeScript: 4 minutes

**By Editor:**
- VS Code: 12 minutes

**Total Time:** 12 minutes
**Total Heartbeats:** 8

## Code References

- **Duration Model**: `models/duration.go`
- **Durations Collection**: `models/durations.go`
- **Duration Service**: `services/duration.go`
- **Summary Service**: `services/summary.go`
- **Heartbeat Model**: `models/heartbeat.go`
- **Duration Repository**: `repositories/duration.go`

## Performance Considerations

1. **Caching is crucial**: Recomputing durations from heartbeats every time is expensive
2. **Batch regeneration**: Every 12 hours is a balance between freshness and load
3. **Streaming heartbeats**: Memory-efficient processing of large datasets
4. **Parallel aggregation**: Multiple summary types computed concurrently
5. **Index optimization**: Time + User indexes on durations table

## Future Improvements

From TODO comments in code:

1. **Multi-interval durations**: Store durations at different heartbeat timeouts (issue #675)
2. **On-the-fly updates**: Update durations as heartbeats flow in, instead of batch regeneration
3. **Better entity tracking**: Improve file-level granularity without sacrificing compression
4. **SQL-based aggregation**: Use native database aggregation for MySQL/PostgreSQL instead of programmatic approach
