# Duration Module - Sequence Diagrams

This document contains sequence diagrams showing the flow of data through the Duration module.

## 1. Heartbeat to Duration Flow (Initial Computation)

```mermaid
sequenceDiagram
    participant Editor as Code Editor
    participant API as Wakapi API
    participant HBS as HeartbeatService
    participant DB as Database
    participant EventBus as Event Bus
    participant Queue as Job Queue
    participant DS as DurationService

    Editor->>API: POST /api/heartbeat
    API->>HBS: Create(heartbeat)
    HBS->>HBS: Validate & Sanitize
    HBS->>DB: INSERT heartbeat
    DB-->>HBS: OK
    HBS->>EventBus: Publish(HeartbeatCreate)
    EventBus-->>DS: Subscribe(HeartbeatCreate)
    
    Note over DS: Check if regeneration needed<br/>(last run > 12 hours)
    
    alt Regeneration Needed
        DS->>Queue: Dispatch Regenerate Job
        Queue->>DS: Execute Regenerate(user, false)
        DS->>DS: Get latest cached duration
        DS->>HBS: StreamAllWithin(latestTime, now)
        HBS->>DB: SELECT heartbeats WHERE...
        DB-->>HBS: Stream heartbeats
        HBS-->>DS: Channel of heartbeats
        DS->>DS: getLive() - Compute durations
        
        loop For each heartbeat
            DS->>DS: Group by timeout & attributes
            DS->>DS: Calculate duration
        end
        
        DS->>DB: InsertBatch(durations)
        DB-->>DS: OK
    end
```

## 2. Summary Generation from Durations

```mermaid
sequenceDiagram
    participant User as User/API Client
    participant SS as SummaryService
    participant DS as DurationService
    participant DR as DurationRepository
    participant HBS as HeartbeatService
    participant DB as Database

    User->>SS: GetSummary(from, to, filters)
    SS->>SS: Aliased() - wrapper
    SS->>SS: Summarize(from, to, user, filters)
    SS->>DS: Get(from, to, user, filters, timeout, skipCache)
    
    alt Cache Hit & No Custom Timeout
        DS->>DR: GetAllWithinByFilters(from, to, filters)
        DR->>DB: SELECT * FROM durations WHERE...
        DB-->>DR: Cached durations
        DR-->>DS: durations[0..n]
        
        alt Missing durations at end
            Note over DS: cached.Last().TimeEnd < to
            DS->>HBS: StreamAllWithin(cached.Last().TimeEnd, to)
            HBS->>DB: SELECT * FROM heartbeats WHERE...
            DB-->>HBS: Stream heartbeats
            DS->>DS: getLive() - Compute missing
            DS->>DS: merge(cached, missing)
        end
    else Cache Miss or Custom Timeout
        DS->>HBS: StreamAllWithin(from, to, user)
        HBS->>DB: SELECT * FROM heartbeats WHERE...
        DB-->>HBS: Stream heartbeats
        DS->>DS: getLive() - Compute all durations
    end
    
    DS-->>SS: durations[]
    
    par Aggregate by entity types
        SS->>SS: aggregateBy(durations, SummaryProject)
        SS->>SS: aggregateBy(durations, SummaryLanguage)
        SS->>SS: aggregateBy(durations, SummaryEditor)
        SS->>SS: aggregateBy(durations, SummaryOS)
        SS->>SS: aggregateBy(durations, SummaryMachine)
        SS->>SS: aggregateBy(durations, SummaryCategory)
        SS->>SS: aggregateBy(durations, SummaryBranch)
        SS->>SS: aggregateBy(durations, SummaryEntity)
    end
    
    SS->>SS: Create Summary with all items
    SS->>SS: Apply aliases & labels
    SS->>SS: Sort & format
    SS-->>User: Summary JSON
```

## 3. Duration Computation Details (getLive)

```mermaid
sequenceDiagram
    participant DS as DurationService
    participant HBS as HeartbeatService
    participant Algo as Duration Algorithm

    DS->>HBS: StreamAllWithin(from, to, user)
    HBS-->>DS: Channel<Heartbeat>
    
    DS->>Algo: Process heartbeat stream
    
    Note over Algo: Initialize:<br/>mapping = {}<br/>latest = nil
    
    loop For each heartbeat H
        Algo->>Algo: Create Duration D from H
        Algo->>Algo: D.GroupHash = Hash(attributes)
        
        alt First heartbeat
            Algo->>Algo: mapping[D.GroupHash] = [D]
            Algo->>Algo: latest = D
        else Subsequent heartbeats
            Algo->>Algo: diff = H.Time - (latest.Time + latest.Duration)
            Algo->>Algo: sameDay = BeginOfDay(H.Time) == BeginOfDay(latest.Time)
            
            alt Should group (diff < timeout AND same hash AND same day)
                Algo->>Algo: latest.Duration += diff
                Algo->>Algo: latest.NumHeartbeats++
                Algo->>Algo: Update most prominent entity
            else Start new group
                Algo->>Algo: mapping[D.GroupHash].append(D)
                Algo->>Algo: latest = D
            end
        end
    end
    
    Algo->>Algo: Flatten mapping to durations[]
    Algo->>Algo: Sort by time ascending
    Algo-->>DS: durations[]
```

## 4. Duration Merging Process

```mermaid
sequenceDiagram
    participant DS as DurationService
    participant Merge as Merge Algorithm

    Note over DS: Have cached[0..n] and<br/>missing[n+1..m] durations
    
    DS->>Merge: merge(cached, missing, user)
    
    alt Empty cases
        Merge->>Merge: if len(cached) == 0: return missing
        Merge->>Merge: if len(missing) == 0: return cached
    end
    
    Merge->>Merge: middleLeft = cached.Last()
    Merge->>Merge: middleRight = missing.First()
    
    alt Overlap detected
        Merge->>Merge: if middleRight.Time < middleLeft.TimeEnd
        Merge-->>DS: Error: overlap
    end
    
    Merge->>Merge: diff = middleRight.Time - middleLeft.TimeEnd
    
    alt diff < HeartbeatTimeout
        alt Same GroupHash
            Note over Merge: Merge into one duration
            Merge->>Merge: merged = copy(middleLeft)
            Merge->>Merge: merged.Duration += diff + middleRight.Duration
            Merge->>Merge: merged.NumHeartbeats += middleRight.NumHeartbeats
            Merge->>Merge: result = cached[0..n-1] + [merged] + missing[n+2..m]
        else Different GroupHash
            Note over Merge: Extend left, keep right
            Merge->>Merge: middleLeft.Duration += diff
            Merge->>Merge: result = cached[0..n] + missing[n+1..m-1]
        end
    else diff >= HeartbeatTimeout
        Note over Merge: Keep both separate
        Merge->>Merge: result = cached + missing
    end
    
    Merge-->>DS: merged durations[]
```

## 5. Regeneration Triggered by Heartbeat

```mermaid
sequenceDiagram
    participant Editor as Code Editor
    participant API as Wakapi API
    participant EventBus as Event Bus
    participant DS as DurationService
    participant Queue as Job Queue
    participant DR as DurationRepository

    Editor->>API: POST /api/heartbeat
    API->>EventBus: Publish(HeartbeatCreate, user)
    EventBus->>DS: Notify HeartbeatCreate
    
    Note over DS: Check lastUserJob[user.ID]
    
    alt Last job > 12 hours ago OR never run
        DS->>Queue: Dispatch Regenerate Job
        Queue->>DS: Regenerate(user, forceAll=false)
        
        alt Already running
            DS->>DS: Check pending.Contains(user.ID)
            Note over DS: Skip - already running
        else Not running
            DS->>DS: pending.Add(user.ID)
            DS->>DR: GetLatestByUser(user)
            DR-->>DS: latest duration
            
            alt Has cached durations
                DS->>DS: from = latest.TimeEnd
            else No cached durations
                DS->>DS: from = time.Zero
            end
            
            DS->>DS: Get(from, now, user, skipCache=forceAll)
            Note over DS: This computes missing durations<br/>using getLive()
            DS-->>DS: durations[]
            
            DS->>DR: InsertBatch(durations)
            DR-->>DS: OK
            DS->>DS: pending.Delete(user.ID)
            DS->>DS: lastUserJob[user.ID] = now
        end
    else Recent job
        Note over DS: Skip - ran recently
    end
```

## 6. Full Regeneration (Admin Operation)

```mermaid
sequenceDiagram
    participant Admin as Admin/CLI
    participant DS as DurationService
    participant US as UserService
    participant DR as DurationRepository
    participant Queue as Job Queue

    Admin->>DS: RegenerateAll()
    DS->>US: GetAll()
    US-->>DS: users[]
    
    loop For each user
        DS->>Queue: Dispatch Regenerate(user, forceAll=true)
        
        Queue->>DS: Execute Regenerate(user, forceAll=true)
        DS->>DS: pending.Add(user.ID)
        
        Note over DS: forceAll=true means delete<br/>all existing and recompute
        
        DS->>DR: DeleteByUser(user)
        DR-->>DS: OK
        
        DS->>DS: Get(time.Zero, now, user, skipCache=true)
        Note over DS: Compute ALL durations<br/>from ALL heartbeats
        DS-->>DS: durations[]
        
        DS->>DR: InsertBatch(durations)
        DR-->>DS: OK
        
        DS->>DS: pending.Delete(user.ID)
    end
    
    DS-->>Admin: Complete
```

## Flow Summary

### Normal Operation
1. **Heartbeat arrives** → Stored in DB → Event published
2. **Event triggers check** → If 12+ hours since last regeneration → Queue job
3. **Background job runs** → Computes new durations from latest cached to now → Stores in DB

### Summary Request
1. **User requests summary** → DurationService.Get() called
2. **Check cache** → If complete, return cached + fill gaps
3. **Compute live** → If cache miss or custom timeout, compute from heartbeats
4. **Aggregate** → SummaryService groups durations by entity types
5. **Return result** → Formatted summary with all statistics

### Key Optimization Points
- **Caching**: Avoid recomputing from heartbeats every time
- **Incremental**: Only compute missing durations, not entire history
- **Streaming**: Process heartbeats in chunks, not loading all into memory
- **Parallel**: Aggregate summary types concurrently
- **Background**: Regeneration happens asynchronously, doesn't block API
