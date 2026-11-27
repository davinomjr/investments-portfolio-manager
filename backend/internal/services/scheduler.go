package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Daily sync scheduler: runs every configured import source (Pluggy, then
// IBKR) at fixed BRT times from SYNC_TIMES, e.g. "08:00,20:00". On startup it
// catches up any source whose last completed sync is older than catchUpAfter,
// so deploys/restarts never cost a day. A failed source is retried once after
// retryDelay; the dashboard badge shows the failure if the retry fails too.

const (
	schedulerStartupDelay = time.Minute
	schedulerCatchUpAfter = 12 * time.Hour
	schedulerRetryDelay   = 30 * time.Minute
)

type syncSource struct {
	name       string
	configured func() error
	run        func(ctx context.Context, jobID int64) error
	timeout    time.Duration
}

type syncScheduler struct {
	svc          *Service
	times        []int // minutes after midnight, BRT
	sources      []syncSource
	now          func() time.Time
	catchUpAfter time.Duration
	mu           sync.Mutex // one pass at a time (schedule, catch-up, retry)
}

// StartSyncScheduler starts the background scheduler when SYNC_TIMES is set.
func (s *Service) StartSyncScheduler(ctx context.Context) {
	if strings.TrimSpace(s.Config.SyncTimes) == "" {
		return
	}
	times, err := parseSyncTimes(s.Config.SyncTimes)
	if err != nil {
		log.Printf("sync scheduler disabled: %v", err)
		return
	}
	sch := s.newSyncScheduler(times)
	names := make([]string, 0, len(sch.sources))
	for _, src := range sch.sources {
		if src.configured() == nil {
			names = append(names, src.name)
		}
	}
	log.Printf("sync scheduler: %s BRT for [%s]", s.Config.SyncTimes, strings.Join(names, ", "))
	go sch.loop(ctx)
}

func (s *Service) newSyncScheduler(times []int) *syncScheduler {
	return &syncScheduler{
		svc:   s,
		times: times,
		sources: []syncSource{
			{name: "pluggy", configured: s.pluggyConfigured, run: s.runPluggyJob, timeout: 5 * time.Minute},
			{name: "ibkr", configured: s.ibkrConfigured, run: s.runIBKRJob, timeout: 10 * time.Minute},
		},
		now:          time.Now,
		catchUpAfter: schedulerCatchUpAfter,
	}
}

func (sch *syncScheduler) loop(ctx context.Context) {
	if !sleepCtx(ctx, schedulerStartupDelay) {
		return
	}
	sch.retryLater(ctx, sch.catchUp(ctx))
	for {
		next := nextSyncTime(sch.now(), sch.times)
		if !sleepCtx(ctx, time.Until(next)) {
			return
		}
		sch.retryLater(ctx, sch.runAll(ctx, sch.sources, "scheduled"))
	}
}

func (sch *syncScheduler) retryLater(ctx context.Context, failed []syncSource) {
	if len(failed) == 0 {
		return
	}
	go func() {
		if sleepCtx(ctx, schedulerRetryDelay) {
			sch.runAll(ctx, failed, "retry")
		}
	}()
}

// catchUp runs every source whose last completed sync is missing or stale.
func (sch *syncScheduler) catchUp(ctx context.Context) []syncSource {
	due := make([]syncSource, 0, len(sch.sources))
	for _, src := range sch.sources {
		last, ok, err := sch.svc.lastCompletedImport(ctx, src.name)
		if err != nil {
			log.Printf("sync scheduler: %s last sync lookup: %v", src.name, err)
			continue
		}
		if !ok || sch.now().Sub(last) > sch.catchUpAfter {
			due = append(due, src)
		}
	}
	if len(due) == 0 {
		return nil
	}
	return sch.runAll(ctx, due, "catch-up")
}

// runAll runs sources one after another and returns the ones that failed.
func (sch *syncScheduler) runAll(ctx context.Context, sources []syncSource, reason string) []syncSource {
	sch.mu.Lock()
	defer sch.mu.Unlock()
	failed := make([]syncSource, 0)
	for _, src := range sources {
		ran, err := sch.runSource(ctx, src, reason)
		switch {
		case err != nil:
			log.Printf("sync scheduler: %s %s sync failed: %v", reason, src.name, err)
			failed = append(failed, src)
		case ran:
			log.Printf("sync scheduler: %s %s sync completed", reason, src.name)
		}
	}
	return failed
}

// runSource runs one source; ran is false when it was skipped.
func (sch *syncScheduler) runSource(ctx context.Context, src syncSource, reason string) (ran bool, err error) {
	if src.configured() != nil {
		return false, nil // not set up on this deployment
	}
	// Skip when a manual/trigger sync of the same source is still running.
	if job, err := sch.svc.GetLatestImportJob(ctx, []string{src.name}); err == nil && job.Status == "running" {
		if started, perr := time.Parse(time.RFC3339, job.UpdatedAt); perr == nil && sch.now().Sub(started) < src.timeout {
			log.Printf("sync scheduler: %s already running (job %d), skipping", src.name, job.ID)
			return false, nil
		}
	}
	jobID, _, err := sch.svc.createJob(ctx, src.name, "running", fmt.Sprintf("Scheduled %s sync started (%s)", src.name, reason))
	if err != nil {
		return false, err
	}
	runCtx, cancel := context.WithTimeout(ctx, src.timeout)
	defer cancel()
	return true, src.run(runCtx, jobID)
}

// parseSyncTimes parses "08:00,20:00" into sorted minutes after midnight.
func parseSyncTimes(raw string) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		hm := strings.SplitN(part, ":", 2)
		if len(hm) != 2 {
			return nil, fmt.Errorf("invalid SYNC_TIMES entry %q (want HH:MM)", part)
		}
		h, herr := strconv.Atoi(hm[0])
		m, merr := strconv.Atoi(hm[1])
		if herr != nil || merr != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return nil, fmt.Errorf("invalid SYNC_TIMES entry %q (want HH:MM)", part)
		}
		if minute := h*60 + m; !seen[minute] {
			seen[minute] = true
			out = append(out, minute)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SYNC_TIMES has no times")
	}
	sort.Ints(out)
	return out, nil
}

// nextSyncTime returns the first scheduled time strictly after now, in BRT.
func nextSyncTime(now time.Time, times []int) time.Time {
	local := now.In(snapshotLocation)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, snapshotLocation)
	for _, minute := range times {
		if t := midnight.Add(time.Duration(minute) * time.Minute); t.After(local) {
			return t
		}
	}
	return midnight.AddDate(0, 0, 1).Add(time.Duration(times[0]) * time.Minute)
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d < 0 {
		d = 0
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
