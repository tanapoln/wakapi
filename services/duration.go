package services

import (
	"errors"
	"log/slog"
	"time"

	"github.com/duke-git/lancet/v2/condition"
	datastructure "github.com/duke-git/lancet/v2/datastructure/set"
	"github.com/duke-git/lancet/v2/datetime"
	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/tuple"
	"github.com/leandro-lugaresi/hub"
	"github.com/muety/artifex/v2"
	"github.com/muety/wakapi/config"
	"github.com/muety/wakapi/models"
	"github.com/muety/wakapi/repositories"
)

const heartbeatPadding = 0 * time.Second
const generateDurationsInterval = 12 * time.Hour

type DurationService struct {
	config                 *config.Config
	eventBus               *hub.Hub
	repository             repositories.IDurationRepository
	heartbeatService       IHeartbeatService
	userService            IUserService
	languageMappingService ILanguageMappingService
	lastUserJob            map[string]time.Time
	queue                  *artifex.Dispatcher
	pending                datastructure.Set[string] // currently running per-user regeneration jobs
	bigQueryService        *BigQueryService
}

func NewDurationService(durationRepository repositories.IDurationRepository, heartbeatService IHeartbeatService, userService IUserService, languageMappingService ILanguageMappingService) *DurationService {
	srv := &DurationService{
		config:                 config.Get(),
		eventBus:               config.EventBus(),
		heartbeatService:       heartbeatService,
		userService:            userService,
		languageMappingService: languageMappingService,
		repository:             durationRepository,
		lastUserJob:            make(map[string]time.Time),
		queue:                  config.GetQueue(config.QueueProcessing),
		pending:                datastructure.New[string](),
	}

	// Initialize BigQuery service if enabled
	if srv.config != nil && srv.config.BigQuery.Enabled {
		bqService, err := NewBigQueryService()
		if err != nil {
			config.Log().Error("failed to initialize bigquery service for durations", "error", err)
		} else {
			srv.bigQueryService = bqService
		}
	}

	// TODO: refactor to updating durations on-the-fly as heartbeats flow in, instead of batch-wise
	sub1 := srv.eventBus.Subscribe(0, config.EventHeartbeatCreate)
	go func(sub *hub.Subscription) {
		for m := range sub.Receiver {
			heartbeat := m.Fields[config.FieldPayload].(*models.Heartbeat)
			user := heartbeat.User

			if t, ok := srv.lastUserJob[user.ID]; !ok || time.Now().Sub(t) > generateDurationsInterval {
				srv.queue.Dispatch(func() {
					srv.Regenerate(user, false)
				})
				srv.lastUserJob[user.ID] = time.Now()
			}
		}
	}(&sub1)

	return srv
}

// Get retrieves durations for a user within a time range using a hybrid caching strategy.
// This is the main entry point for getting duration data.
//
// Strategy:
// 1. If custom timeout or skipCache requested → compute live from heartbeats
// 2. Otherwise, try to use cached durations from database
// 3. If cache incomplete (missing data at end) → compute missing part live
// 4. Merge cached and live durations intelligently
// 5. Apply filters and return result
//
// Parameters:
//   - from, to: Time range to query
//   - user: User to get durations for
//   - filters: Optional filters to apply (project, language, etc.)
//   - customTimeout: Optional custom heartbeat timeout (overrides user's setting)
//   - skipCache: Force recomputation from heartbeats
//
// Returns: Array of durations sorted by time, filtered according to filters
func (srv *DurationService) Get(from, to time.Time, user *models.User, filters *models.Filters, customTimeout *time.Duration, skipCache bool) (durations models.Durations, err error) {
	// note about "multi-level" durations at different intervals:
	// while durations themselves store the interval (aka. heartbeats timeout) they were computed for, we currently don't support actually storing durations at different intervals
	// if an interval different from the user's preference is requested, recompute durations live from heartbeats and skip cache
	effectiveTimeout := getEffectiveTimeout(user, customTimeout)
	skipCache = skipCache || effectiveTimeout != user.HeartbeatsTimeout()

	// Path 1: Recompute live (bypass cache)
	// This happens when:
	// - skipCache is explicitly requested
	// - A custom timeout different from user's preference is requested
	if skipCache {
		durations, err = srv.getLive(from, to, user, effectiveTimeout)
		if err != nil {
			return nil, err
		}
		return srv.filter(durations, user, filters), nil
	}

	// Path 2: Use cache (optimal path)
	// Try to get cached durations from database
	cached, err := srv.getCached(from, to, user, filters)
	if err != nil {
		config.Log().Error("failed to get cached durations", "user", user.ID, "from", from, "to", to, "error", err)
		cached = models.Durations{}
	}

	// Check if cache is complete or has gaps at the end
	// fill missing
	// for simplicity, we assume no missing durations before 'from' or between 'from' and 'to'
	if len(cached) == 0 || cached.Last().TimeEnd().Before(to) {
		// Calculate where to start computing missing durations
		from := from
		if len(cached) > 0 {
			// We have some cached data, compute only from where cache ends
			from = cached.Last().TimeEnd().Add(time.Second)
		}

		// Compute the missing durations live from heartbeats
		missing, err := srv.getLive(from, to, user, effectiveTimeout)
		if err != nil {
			return nil, err
		}
		
		// Intelligently merge cached and newly computed durations
		durations, err = srv.merge(cached, missing, user)
		if err != nil {
			return nil, err
		}
	} else {
		// Cache is complete, use it as-is
		durations = append(durations, cached...)
	}

	return srv.filter(durations, user, filters), nil
}

// Regenerate recomputes and caches durations for a user.
// This is called periodically (every 12 hours) or manually to update the duration cache.
//
// Algorithm:
// 1. Check if regeneration is already running for this user (prevent duplicates)
// 2. Find the latest cached duration to determine where to start
// 3. Compute new durations from that point until now
// 4. Store the new durations in the database cache
//
// Parameters:
//   - user: User to regenerate durations for
//   - forceAll: If true, delete all cached durations and recompute everything
//
// This runs in a background job queue and doesn't block API requests.
func (srv *DurationService) Regenerate(user *models.User, forceAll bool) {
	if srv.pending.Contain(user.ID) {
		config.Log().Warn("skipping regeneration of durations for user, because already running", "user", user.ID)
		return
	}
	srv.pending.Add(user.ID)
	defer srv.pending.Delete(user.ID)

	var from time.Time
	latest, err := srv.repository.GetLatestByUser(user)
	if err == nil && latest != nil && !forceAll {
		from = latest.TimeEnd()
	}

	slog.Info("generating ephemeral durations for user up until now", "user", user.ID, "from", from)

	durations, err := srv.Get(from, time.Now(), user, nil, nil, forceAll)
	if err != nil {
		config.Log().Error("failed to regenerate ephemeral durations for user up until now", "user", user.ID, "error", err)
		return
	}
	if len(durations) > 0 && durations[0].Time.T().Before(from) && !forceAll {
		config.Log().Warn("got generated duration before requested min date", "user", user.ID, "time", durations[0].Time.T(), "group_hash", durations[0].GroupHash, "min_date", from)
	}

	if forceAll {
		if err := srv.repository.DeleteByUser(user); err != nil {
			config.Log().Error("failed to delete old durations while generating ephemeral new ones", "user", user.ID, "error", err)
			return
		}
	}

	if err := srv.repository.InsertBatch(durations); err != nil {
		config.Log().Error("failed to persist new ephemeral durations for user", "user", user.ID, "error", err)
		return
	}

	// Write to BigQuery asynchronously after successful database insert
	if srv.bigQueryService != nil && len(durations) > 0 {
		go func(durs []*models.Duration) {
			if err := srv.bigQueryService.InsertDurations(durs); err != nil {
				config.Log().Error("failed to insert durations to bigquery", "user", user.ID, "error", err)
			}
		}(durations)
	}
}

func (srv *DurationService) RegenerateAll() {
	slog.Info("regenerating all durations for all users, this may take a long while")

	users, err := srv.userService.GetAll()
	if err != nil {
		config.Log().Error("failed to fetch users for durations regeneration", "error", err)
		return
	}

	for _, u := range users {
		srv.queue.Dispatch(func() {
			srv.Regenerate(u, true)
		})
	}
}

func (srv *DurationService) DeleteByUser(user *models.User) error {
	return srv.repository.DeleteByUser(user)
}

func (srv *DurationService) getCached(from, to time.Time, user *models.User, filters *models.Filters) (models.Durations, error) {
	languageMappings, err := srv.languageMappingService.ResolveByUser(user.ID)
	if err != nil {
		return nil, err
	}
	durations, err := srv.repository.GetAllWithinByFilters(from, to, user, srv.filtersToColumnMap(filters))
	if err != nil {
		return nil, err
	}
	return models.Durations(durations).Augmented(languageMappings).Sorted(), nil
}

// getLive computes durations from raw heartbeats in real-time.
// This is the core algorithm that transforms a stream of heartbeats into durations.
//
// Algorithm Overview:
// 1. Stream heartbeats chronologically from the database
// 2. Group consecutive heartbeats that:
//    - Are within the heartbeat timeout (default 10 minutes)
//    - Have the same attributes (project, language, editor, etc.)
//    - Occur on the same day
// 3. Calculate duration from time gaps between heartbeats in each group
// 4. Return sorted array of durations
//
// Example: If you code for 30 minutes with heartbeats every 2 minutes,
// this creates 1 duration with Duration=30min and NumHeartbeats=16.
func (srv *DurationService) getLive(from, to time.Time, user *models.User, interval time.Duration) (models.Durations, error) {
	heartbeatsTimeout := interval

	heartbeats, err := srv.heartbeatService.StreamAllWithin(from, to, user)
	if err != nil {
		return nil, err
	}

	// Aggregation
	// The below logic is approximately (no filtering, no "same day"-check) equivalent to the SQL query at scripts/aggregate_durations_mysql.sql.
	// A Postgres-compatible script was contributed by @cwilby and is available at scripts/aggregate_durations_postgres.sql
	// I'm hesitant to replicate that logic for sqlite (because probably painful to impossible), but we could think about
	// adding a distinction here to use pure-sql aggregation for MySQL and Postgres, and traditional, programmatic aggregation for all other databases.
	var count int
	var latest *models.Duration

	// mapping stores durations by their GroupHash (hash of project, language, editor, etc.)
	// This allows O(1) lookup to find the right duration group for each heartbeat
	mapping := make(map[string][]*models.Duration)
	
	// entityDurations tracks time spent on each file within a duration group
	// Used to determine the "most prominent" file for each duration
	entityDurations := make(map[tuple.Tuple2[string, string]]time.Duration)

	// Process each heartbeat sequentially
	// Heartbeats are streamed in chronological order
	for h := range heartbeats {
		if h.User == nil {
			h.User = user
		}

		// Create a Duration object from this heartbeat
		// - WithEntityIgnored(): Don't include file name in GroupHash (reduces cardinality)
		// - WithTimeout(): Set the heartbeat timeout to use
		// - Hashed(): Compute GroupHash from attributes (project, language, editor, etc.)
		d1 := models.NewDurationFromHeartbeat(h).WithEntityIgnored().WithTimeout(interval).Hashed()

		// initialize map entry
		if list, ok := mapping[d1.GroupHash]; !ok || len(list) < 1 {
			mapping[d1.GroupHash] = []*models.Duration{}
		}

		// first heartbeat - initialize the first duration
		if latest == nil {
			mapping[d1.GroupHash] = append(mapping[d1.GroupHash], d1)
			latest = d1
			continue
		}

		// Calculate time difference from the last heartbeat
		// Skip heartbeats that span across two adjacent summaries (assuming there are no more than 1 summary per day).
		// This is relevant to prevent the time difference between generating summaries from raw heartbeats and aggregating pre-generated summaries.
		// For the latter case, the very last heartbeat of a day won't be counted, so we don't want to count it here either
		sameDay := datetime.BeginOfDay(d1.Time.T()) == datetime.BeginOfDay(latest.Time.T())
		
		// Calculate the gap between this heartbeat and the end of the previous duration
		// If not same day, gap is treated as 0 (won't be added to duration)
		dur := condition.Ternary[bool, time.Duration](sameDay, d1.Time.T().Sub(latest.Time.T().Add(latest.Duration)), 0)
		
		// Add the gap to the latest duration, but only if it's within the heartbeat timeout
		// Otherwise, add heartbeatPadding (0 seconds) to not extend the duration
		latest.Duration += condition.Ternary[bool, time.Duration](dur < heartbeatsTimeout, dur, heartbeatPadding)

		// Decide whether to continue the current duration or start a new one
		// Start new "group" if:
		// (a) heartbeats were too far apart each other or,
		// (b) they are of a different entity or,
		// (c) they span across two days
		if dur >= heartbeatsTimeout || latest.GroupHash != d1.GroupHash || !sameDay {
			// Start a new duration - this heartbeat is too different or too far from the previous one
			mapping[d1.GroupHash] = append(mapping[d1.GroupHash], d1)
			latest = d1
		} else {
			// Continue the current duration - this heartbeat is part of the same coding session
			latest.NumHeartbeats++
			// TODO: think about how to fix this properly
			// Problem: we don't consider entities (aka. file names) when distinguishing between durations, that is, in other words,
			// durations essentially aggregate heartbeats by (a) by time (squash all heartbeats within <heartbeatsTimeout> and (b) by entity.
			// This conflicts with file-level summaries (as shown on the project details page), because fine-grained file information is lost.
			// As a heuristic, for each duration we keep the file (entity) that was "most prominent" (i.e. was coded on most) with the duration.
			// However, when you code for 10 minutes straight, where you work 6 minutes on file A and 4 minutes on file B, the latter still won't show up in the summary at all.
			// This is a tricky trade-off, because if we changed durations to respect entities as well, we'd end up with much higher cardinality, thus much less "compression" in comparison to raw heartbeats and thus higher compute- and storage load.
			latest = updateDurationEntity(latest, h, entityDurations) // dirty hack
		}

		count++
	}

	// Flatten the mapping into a single array of durations
	durations := slice.Flatten(maputil.Values[string, []*models.Duration](mapping)).([]*models.Duration)

	// Special case: single heartbeat should have 0 duration
	if len(heartbeats) == 1 && len(durations) == 1 {
		durations[0].Duration = heartbeatPadding
	}

	// note: no need to do language augmentation here, because already done while retrieving heartbeats
	return models.Durations(durations).Sorted(), nil
}

func (srv *DurationService) filter(durations []*models.Duration, user *models.User, filters *models.Filters) models.Durations {
	filtered := make([]*models.Duration, 0, len(durations))

	for _, d := range durations {
		// Even when filters are applied, we'll still have to compute the whole summary first and then filter out non-matching durations.
		// If we fetched only matching heartbeats in the first place, there will be false positive gaps (see heartbeatsTimeout)
		// in case the user worked on different projects in parallel.
		// See https://github.com/muety/wakapi/issues/535, https://github.com/muety/wakapi/issues/716
		if filters != nil && !filters.MatchDuration(d) {
			continue
		}
		if user.ExcludeUnknownProjects && d.Project == "" {
			continue
		}

		filtered = append(filtered, d)
	}

	return filtered
}

// merge combines cached durations with newly computed durations intelligently.
// This allows incremental updates without recomputing the entire history.
//
// Algorithm:
// 1. Take all cached durations except the last one
// 2. Try to merge the last cached duration with the first new duration
// 3. Append remaining new durations
//
// Merge logic at the boundary:
// - If gap < heartbeat timeout AND same attributes → merge into one
// - If gap < heartbeat timeout BUT different attributes → extend left, keep right
// - If gap >= heartbeat timeout → keep both separate
//
// Example:
//   cached: [D1: 00:00-00:10, D2: 00:15-00:25]
//   missing: [D3: 00:27-00:35]
//   gap: 2 minutes (00:25 to 00:27)
//   If gap < timeout AND D2.hash == D3.hash → merge D2 and D3
//   Result: [D1, D2-merged-with-D3]
func (srv *DurationService) merge(d1, d2 models.Durations, user *models.User) (models.Durations, error) {
	if len(d1) == 0 {
		return d2, nil
	}
	if len(d2) == 0 {
		return d1, nil
	}

	// d1 and d2 are assumed to be sorted by time and distinct
	middleLeft, middleRight := d1.Last(), d2.First()
	if middleRight.Time.T().Before(middleLeft.TimeEnd()) {
		return nil, errors.New("failed to merge durations due to overlap")
	}

	merged := make(models.Durations, 0, len(d1)+len(d2))
	merged = append(merged, d1[0:len(d1)-1]...)

	if diff := middleRight.Time.T().Sub(middleLeft.TimeEnd()); diff < user.HeartbeatsTimeout() {
		if middleLeft.GroupHash == middleRight.GroupHash {
			middleMerged := &(*middleLeft)
			middleMerged.Duration += diff + middleRight.Duration
			middleMerged.NumHeartbeats += middleRight.NumHeartbeats
			middleMerged.Hashed()
			merged = append(merged, middleMerged) // left and right are merged into one
		} else {
			middleLeft.Duration += diff
			middleLeft.Hashed()
			merged = append(merged, middleLeft) // left is extended, right is kept
		}
	} else {
		merged = append(merged, middleLeft, middleRight) // both are kept as is
	}

	if len(d2) > 1 {
		merged = append(merged, d2[1:len(d2)-1]...)
	}
	return merged, nil
}

func (srv *DurationService) filtersToColumnMap(filters *models.Filters) map[string][]string {
	columnMap := map[string][]string{}

	if filters == nil {
		return columnMap
	}

	for _, t := range models.NativeSummaryTypes() {
		f := filters.ResolveType(t)
		if len(*f) > 0 {
			columnMap[models.GetEntityColumn(t)] = *f
		}
	}

	return columnMap
}

func getEffectiveTimeout(user *models.User, overrideTimeout *time.Duration) time.Duration {
	if overrideTimeout == nil {
		return user.HeartbeatsTimeout()
	}
	return *overrideTimeout
}

func updateDurationEntity(d *models.Duration, h *models.Heartbeat, entityDurations map[tuple.Tuple2[string, string]]time.Duration) *models.Duration {
	// check if total time for the entity of the given heartbeat exceeds the duration's entity's total time
	// if yes, update duration entity so that it always reflect the "most prominent" entity of this group
	key1 := tuple.NewTuple2(d.GroupHash, h.Entity)
	key2 := tuple.NewTuple2(d.GroupHash, d.Entity)
	if _, ok := entityDurations[key1]; !ok {
		entityDurations[key1] = time.Duration(0)
	}
	entityDurations[key1] += d.Duration
	dur1 := entityDurations[key1]
	dur2 := entityDurations[key2]
	if dur1 > dur2 {
		d.Entity = h.Entity
	}
	return d
}
