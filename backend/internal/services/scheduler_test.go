package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"investments-portfolio-manager/backend/internal/config"
)

func TestParseSyncTimes(t *testing.T) {
	cases := []struct {
		raw     string
		want    []int
		wantErr bool
	}{
		{"08:00,20:00", []int{480, 1200}, false},
		{" 20:00 , 8:00 ,08:00", []int{480, 1200}, false},
		{"23:59", []int{1439}, false},
		{"24:00", nil, true},
		{"8h", nil, true},
		{"08:60", nil, true},
		{" , ", nil, true},
	}
	for _, c := range cases {
		got, err := parseSyncTimes(c.raw)
		if (err != nil) != c.wantErr {
			t.Errorf("parseSyncTimes(%q) err=%v wantErr=%v", c.raw, err, c.wantErr)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseSyncTimes(%q)=%v want %v", c.raw, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseSyncTimes(%q)=%v want %v", c.raw, got, c.want)
			}
		}
	}
}

func TestNextSyncTime(t *testing.T) {
	times := []int{8 * 60, 20 * 60}
	brt := snapshotLocation
	cases := []struct {
		now  time.Time
		want time.Time
	}{
		{time.Date(2026, 11, 26, 7, 59, 0, 0, brt), time.Date(2026, 11, 26, 8, 0, 0, 0, brt)},
		{time.Date(2026, 11, 26, 8, 0, 0, 0, brt), time.Date(2026, 11, 26, 20, 0, 0, 0, brt)},
		{time.Date(2026, 11, 26, 20, 1, 0, 0, brt), time.Date(2026, 11, 27, 8, 0, 0, 0, brt)},
		// UTC input: 23:30 UTC is 20:30 BRT → next is 08:00 BRT tomorrow.
		{time.Date(2026, 11, 26, 23, 30, 0, 0, time.UTC), time.Date(2026, 11, 27, 8, 0, 0, 0, brt)},
		// Month rollover.
		{time.Date(2026, 11, 30, 21, 0, 0, 0, brt), time.Date(2026, 12, 1, 8, 0, 0, 0, brt)},
	}
	for _, c := range cases {
		if got := nextSyncTime(c.now, times); !got.Equal(c.want) {
			t.Errorf("nextSyncTime(%v)=%v want %v", c.now, got, c.want)
		}
	}
}

type fakeSource struct {
	calls int
	err   error
}

func (f *fakeSource) source(svc *Service, name string, configured bool) syncSource {
	return syncSource{
		name: name,
		configured: func() error {
			if !configured {
				return errors.New("not configured")
			}
			return nil
		},
		run: func(ctx context.Context, jobID int64) error {
			f.calls++
			status, detail := "completed", "ok"
			if f.err != nil {
				status, detail = "failed", f.err.Error()
			}
			svc.updateJob(ctx, jobID, status, detail)
			return f.err
		},
		timeout: time.Minute,
	}
}

func TestSchedulerRunAll(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ok, bad, off := &fakeSource{}, &fakeSource{err: errors.New("boom")}, &fakeSource{}
	sch := svc.newSyncScheduler([]int{480})
	sch.sources = []syncSource{ok.source(svc, "pluggy", true), bad.source(svc, "ibkr", true), off.source(svc, "other", false)}

	failed := sch.runAll(context.Background(), sch.sources, "scheduled")
	if ok.calls != 1 || bad.calls != 1 || off.calls != 0 {
		t.Fatalf("calls: ok=%d bad=%d off=%d", ok.calls, bad.calls, off.calls)
	}
	if len(failed) != 1 || failed[0].name != "ibkr" {
		t.Fatalf("failed: %+v", failed)
	}
	job, err := svc.GetLatestImportJob(context.Background(), []string{"pluggy"})
	if err != nil || job.Status != "completed" {
		t.Fatalf("pluggy job: %+v %v", job, err)
	}
}

func TestSchedulerSkipsSourceAlreadyRunning(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()
	if _, _, err := svc.createJob(ctx, "pluggy", "running", "manual sync"); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{}
	sch := svc.newSyncScheduler([]int{480})
	sch.sources = []syncSource{src.source(svc, "pluggy", true)}
	sch.runAll(ctx, sch.sources, "scheduled")
	if src.calls != 0 {
		t.Fatalf("should skip while a recent job is running, ran %d times", src.calls)
	}

	// A "running" job older than the timeout is treated as dead.
	sch.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	sch.runAll(ctx, sch.sources, "scheduled")
	if src.calls != 1 {
		t.Fatalf("stale running job should not block, ran %d times", src.calls)
	}
}

func TestSchedulerCatchUp(t *testing.T) {
	svc := newTestServiceWithConfig(t, config.Config{DefaultUserID: 1})
	ctx := context.Background()
	recent, stale, never := &fakeSource{}, &fakeSource{}, &fakeSource{}
	sch := svc.newSyncScheduler([]int{480})
	sch.sources = []syncSource{recent.source(svc, "pluggy", true), stale.source(svc, "ibkr", true), never.source(svc, "other", true)}

	for _, name := range []string{"pluggy", "ibkr"} {
		id, _, err := svc.createJob(ctx, name, "running", "")
		if err != nil {
			t.Fatal(err)
		}
		svc.updateJob(ctx, id, "completed", "")
	}
	// Pretend 13h passed for ibkr only by shifting the clock and making
	// pluggy's last sync fresh again.
	sch.now = func() time.Time { return time.Now().Add(13 * time.Hour) }
	id, _, _ := svc.createJob(ctx, "pluggy", "running", "")
	if _, err := svc.DB.ExecContext(ctx, `UPDATE import_jobs SET status='completed', updated_at=? WHERE id=?`, time.Now().Add(12*time.Hour).UTC().Format(time.RFC3339), id); err != nil {
		t.Fatal(err)
	}

	sch.catchUp(ctx)
	if recent.calls != 0 || stale.calls != 1 || never.calls != 1 {
		t.Fatalf("catch-up calls: recent=%d stale=%d never=%d", recent.calls, stale.calls, never.calls)
	}
}
