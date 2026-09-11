package trigger

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/meshcore-go/OwlShack/internal/config"
)

func TestCAPTrigger_DecoratesFromLinkedAlert(t *testing.T) {
	t.Parallel()
	alert, err := os.ReadFile("testdata/cap_alert.xml")
	if err != nil {
		t.Fatal(err)
	}
	fs := newFeedServer(t)
	body := string(alert)
	fs.alertBody.Store(&body)

	tr, fired := newTestCAP(t, config.TriggerConfig{URL: fs.feedURL()})
	tr.poll(context.Background())

	fs.publish("severe-weather")
	tr.poll(context.Background())

	if len(*fired) != 1 {
		t.Fatalf("fired %d events, want 1", len(*fired))
	}
	data := (*fired)[0].Data
	for field, want := range map[string]string{
		"Headline":   "Heavy Rain Warning",
		"Severity":   "Moderate",
		"Urgency":    "Immediate",
		"Certainty":  "Likely",
		"MsgType":    "Update",
		"Status":     "Actual",
		"Event":      "rain",
		"SenderName": "Meteorological Service of New Zealand Limited",
	} {
		if got, _ := data[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if areas, _ := data["Areas"].(string); !strings.Contains(areas, "Northland") {
		t.Errorf("Areas = %q, want it to name Northland", areas)
	}
	if expires, _ := data["Expires"].(time.Time); expires.IsZero() {
		t.Error("Expires is zero; templates cannot show when an alert lapses")
	}
}

func TestCAPTrigger_UnfetchableAlertIsNotRetriedForever(t *testing.T) {
	t.Parallel()
	fs := newFeedServer(t)
	fs.alertCode.Store(http.StatusNotFound)

	tr, fired := newTestCAP(t, config.TriggerConfig{URL: fs.feedURL()})
	tr.poll(context.Background())

	fs.publish("gone")
	tr.poll(context.Background())
	tr.poll(context.Background())
	tr.poll(context.Background())

	if len(*fired) != 0 {
		t.Fatalf("fired %d events for an alert that cannot be read", len(*fired))
	}
	if hits := fs.alertHits.Load(); hits != 1 {
		t.Fatalf("fetched the dead alert %d times, want 1: a 404 will not become an alert", hits)
	}
}

func TestCAPTrigger_TransientFailureIsRetried(t *testing.T) {
	t.Parallel()
	fs := newFeedServer(t)
	fs.alertCode.Store(http.StatusServiceUnavailable)

	tr, fired := newTestCAP(t, config.TriggerConfig{URL: fs.feedURL()})
	tr.poll(context.Background())

	fs.publish("flaky")
	tr.poll(context.Background())
	if len(*fired) != 0 {
		t.Fatalf("fired despite a 503")
	}

	alert, err := os.ReadFile("testdata/cap_alert.xml")
	if err != nil {
		t.Fatal(err)
	}
	body := string(alert)
	fs.alertBody.Store(&body)
	fs.alertCode.Store(http.StatusOK)

	tr.poll(context.Background())
	if len(*fired) != 1 {
		t.Fatalf("fired %d events once the publisher recovered, want 1", len(*fired))
	}
}
