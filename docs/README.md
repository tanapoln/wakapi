# Wakapi Duration Module Documentation

Welcome to the comprehensive documentation for Wakapi's Duration Computation module. This documentation explains how raw heartbeat data from code editors is transformed into meaningful time summaries.

## Documentation Overview

### 📚 Main Documents

1. **[DURATION_COMPUTATION.md](./DURATION_COMPUTATION.md)** - Core Documentation
   - Overview of the Duration module
   - Key concepts (Heartbeats, Durations, Timeouts)
   - Architecture diagram
   - Detailed algorithm explanations
   - Caching strategy
   - Example walkthrough
   - Performance considerations

2. **[DURATION_SEQUENCE_DIAGRAMS.md](./DURATION_SEQUENCE_DIAGRAMS.md)** - Sequence Diagrams
   - Heartbeat to Duration flow
   - Summary generation from durations
   - Duration computation details
   - Duration merging process
   - Regeneration triggers
   - Full regeneration workflow

3. **[DURATION_DATA_FLOW.md](./DURATION_DATA_FLOW.md)** - Data Flow & Transformations
   - Data structure evolution (Heartbeat → Duration → Summary)
   - Detailed transformations
   - Caching flow
   - Storage efficiency analysis
   - Performance characteristics

## Quick Start Guide

### For Users

If you're just trying to understand how Wakapi tracks your coding time:

1. Your **code editor** sends heartbeats every ~2 minutes while you code
2. Wakapi groups these heartbeats into **durations** based on:
   - Time gaps between heartbeats (default: 10 minute timeout)
   - What you're working on (project, language, etc.)
3. Durations are aggregated into **summaries** showing:
   - Time per project, language, editor, etc.
   - Total coding time
   - Activity breakdown

**Example**: If you work on "project A" for 30 minutes, then switch to "project B" for 20 minutes, Wakapi creates 2 durations and shows you spent 50 minutes total (30 on A, 20 on B).

### For Developers

If you're looking to understand or modify the Duration module:

1. **Start with**: [DURATION_COMPUTATION.md](./DURATION_COMPUTATION.md) - Read the "Key Concepts" and "Duration Computation Algorithm" sections
2. **Then review**: [DURATION_SEQUENCE_DIAGRAMS.md](./DURATION_SEQUENCE_DIAGRAMS.md) - See how data flows through the system
3. **Finally check**: [DURATION_DATA_FLOW.md](./DURATION_DATA_FLOW.md) - Understand data transformations and storage

Key source files to explore:
- `models/heartbeat.go` - Heartbeat data structure
- `models/duration.go` - Duration data structure
- `services/duration.go` - Duration computation logic
- `services/summary.go` - Summary aggregation logic

## Core Concepts

### Heartbeat
A single data point sent by your code editor plugin, containing:
- Timestamp
- File being edited
- Project name
- Programming language
- Editor, OS, machine info

### Duration
An aggregated time span from consecutive heartbeats:
- Start time
- Duration length (computed)
- Project, language, editor info
- Number of heartbeats included

### Summary
Final aggregated statistics:
- Time per project
- Time per language
- Time per editor/OS/machine
- Total time and heartbeat count

## How Duration is Recomputed

### Automatic Regeneration

Durations are automatically recomputed:

1. **Every 12 hours** per user after they send a heartbeat
2. **On-demand** when requesting data with custom settings
3. **Incrementally** - only missing durations are computed, not entire history

### Manual Regeneration

Administrators can trigger full regeneration:
- CLI command or API call
- Recomputes all durations from all heartbeats
- Useful after timeout changes or data fixes

### Why Caching Matters

- Computing durations from raw heartbeats is expensive (O(n) where n = heartbeats)
- Cached durations provide ~10x storage compression vs heartbeats
- Hybrid approach: Use cache + fill gaps live for best performance

## Algorithm Overview

### Grouping Algorithm (Heartbeat → Duration)

```
1. Start with first heartbeat
2. For each subsequent heartbeat:
   a. Calculate time difference from previous
   b. If diff < timeout AND same attributes → Add to current duration
   c. Else → Start new duration
3. Result: Array of durations with computed time spans
```

### Merging Algorithm (Cached + Live Durations)

```
1. Get cached durations from database
2. Identify gaps (missing time ranges)
3. Compute missing durations live from heartbeats
4. Intelligently merge:
   - If adjacent durations are similar → merge them
   - If different or far apart → keep separate
5. Result: Complete duration set for requested time range
```

### Aggregation Algorithm (Duration → Summary)

```
1. For each entity type (project, language, editor, etc.):
   a. Group durations by that entity
   b. Sum total time for each group
   c. Sort by time descending
2. Combine all entity types into summary
3. Apply filters, aliases, labels
4. Result: Complete summary with all statistics
```

## Important Design Decisions

### Why Exclude Files from Grouping?

By default, individual file names don't affect duration grouping:
- **Pro**: Reduces storage, one duration per coding session
- **Con**: File-level details are approximated
- **Result**: Good balance between compression and detail

### Why 10 Minute Timeout?

The default 10-minute timeout balances:
- **Too short**: Many small durations, less compression
- **Too long**: Unrelated activities grouped together
- **10 minutes**: Works well for typical coding patterns

### Why Cache Durations?

Raw heartbeats are expensive to process:
- **Problem**: Recomputing from heartbeats every query is slow
- **Solution**: Cache computed durations, fill gaps on demand
- **Benefit**: Fast queries while staying up-to-date

## Performance Tips

For developers working with the Duration module:

1. **Use cache when possible** - Set `skipCache=false` unless you need custom timeout
2. **Batch operations** - Background jobs handle regeneration, don't do it synchronously
3. **Stream heartbeats** - Don't load all into memory
4. **Parallelize aggregation** - Summary types can be computed concurrently
5. **Index properly** - Ensure `time + user_id` indexes on durations table

## Troubleshooting

### Durations seem incorrect

- Check user's heartbeat timeout setting
- Verify heartbeats are being received
- Check if regeneration job is running
- Look for gaps in heartbeat data

### Slow summary generation

- Check if cache is being used
- Verify duration indexes exist
- Look for large time ranges without cached durations
- Consider pre-computing summaries for common queries

### Missing time in summaries

- Check filter settings
- Verify heartbeats were sent during that time
- Check for "unknown project" exclusion setting
- Verify day boundaries (durations don't cross days)

## Further Reading

- WakaTime API compatibility: See `/routes` and `/models/compat`
- Database migrations: See `/migrations`
- Testing: See `/services/*_test.go`
- Repository patterns: See `/repositories`

## Contributing

If you're modifying the Duration module:

1. Read all three documentation files completely
2. Understand the test suite in `services/duration_test.go`
3. Consider performance implications of changes
4. Update documentation if behavior changes
5. Add tests for new functionality

## Questions?

- Check existing code comments in the source files
- Review test cases for examples
- Open an issue on GitHub for clarification
- Contact the maintainers

---

**Last Updated**: November 2025
**Wakapi Version**: 2.x
**Documentation Status**: ✅ Complete
