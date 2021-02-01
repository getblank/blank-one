package internet

import (
	"net/http/httptest"
	"testing"
	"time"
)

type clockStub struct{}

func (clockStub) Now() time.Time { return time.Now() }
func (clockStub) Since(t time.Time) time.Duration {
	duration, _ := time.ParseDuration("2s")
	return duration
}

func TestTiming(t *testing.T) {
	clock = clockStub{}
	w := httptest.NewRecorder()
	timing := newServerTiming(w, "anyTiming")

	timing.End()

	header := w.Header().Get("Server-Timing")
	expected := "anyTiming;dur=2.000000"
	if header != expected {
		t.Fatalf("Server-Timing is %s, expected: %s", header, expected)
	}
}
