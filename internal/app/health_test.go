package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/meshcore-go/OwlShack/internal/store"
)

func newHealthBackend(t *testing.T) *backend {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// Nil stats and no nodes is the shape of a process whose radio never came up — the case the
	// endpoint exists to make visible.
	return newBackend(nil, nil, db, nil, nil, nil, nil, nil)
}

func problemSet(t *testing.T, b *backend, now time.Time, act *radioActivity) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, p := range b.health(now, act).Problems {
		set[p] = true
	}
	return set
}

func TestHealth_NoRadioIsReportedNotZeroed(t *testing.T) {
	t.Parallel()
	b := newHealthBackend(t)
	info := b.health(time.Now(), &radioActivity{})

	if info.Radio.Connected {
		t.Fatal("Radio.Connected true with no modem")
	}
	// The counters must not read as a healthy quiet radio, and the ages must say "cannot measure"
	// rather than "just now".
	if info.Radio.LastRxSecs != nil || info.Radio.LastTxSecs != nil || info.Radio.LastReplySecs != nil {
		t.Errorf("ages should be null before any traffic, got rx=%v tx=%v reply=%v",
			info.Radio.LastRxSecs, info.Radio.LastTxSecs, info.Radio.LastReplySecs)
	}
	if info.Radio.Transport != "" {
		t.Errorf("Transport = %q with no modem", info.Radio.Transport)
	}

	problems := problemSet(t, b, time.Now(), &radioActivity{})
	for _, want := range []string{"radio: modem not connected", "no companion or repeater node is running"} {
		if !problems[want] {
			t.Errorf("missing problem %q, got %v", want, problems)
		}
	}
}

func TestHealth_ReportsTrafficAges(t *testing.T) {
	t.Parallel()
	b := newHealthBackend(t)
	now := time.Now()

	var act radioActivity
	act.lastRx.Store(now.Add(-90 * time.Second).UnixNano())
	act.lastTx.Store(now.Add(-5 * time.Second).UnixNano())

	info := b.health(now, &act)
	if info.Radio.LastRxSecs == nil || *info.Radio.LastRxSecs != 90 {
		t.Errorf("LastRxSecs = %v, want 90", info.Radio.LastRxSecs)
	}
	if info.Radio.LastTxSecs == nil || *info.Radio.LastTxSecs != 5 {
		t.Errorf("LastTxSecs = %v, want 5", info.Radio.LastTxSecs)
	}
	// An age is a fact for the operator to threshold, never a verdict here: 90s of silence on a
	// quiet mesh is normal, and nothing in this endpoint may decide otherwise.
	if problemSet(t, b, now, &act)["radio: silent"] {
		t.Error("mesh silence was turned into a problem")
	}
}

func TestHealth_DroppedWritesSurface(t *testing.T) {
	t.Parallel()
	b := newHealthBackend(t)

	if problemSet(t, b, time.Now(), &radioActivity{})["database: 1 writes dropped, the write queue overflowed"] {
		t.Fatal("reported dropped writes before any were dropped")
	}

	// Fill the queue behind a blocked writer, then overflow it: a dropped write is invisible
	// everywhere else, which is the whole reason it is published here.
	release := make(chan struct{})
	b.db.WriteAsync(func() { <-release })
	for i := 0; i < 4096; i++ {
		b.db.WriteAsync(func() {})
	}
	defer close(release)

	info := b.health(time.Now(), &radioActivity{})
	if info.Database.WritesDropped == 0 {
		t.Fatalf("no writes recorded as dropped; queue len %d of %d",
			info.Database.WriteQueueLen, info.Database.WriteQueueCap)
	}
	if len(info.Problems) == 0 {
		t.Fatal("dropped writes produced no problem entry")
	}
	found := false
	for _, p := range info.Problems {
		if len(p) > 9 && p[:9] == "database:" {
			found = true
		}
	}
	if !found {
		t.Errorf("no database problem in %v", info.Problems)
	}
}
