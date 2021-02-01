package internet

import (
	"fmt"
	"net/http"
	"time"
)

type Clock interface {
	Now() time.Time
	Since(time.Time) time.Duration
}

type clockImplementation struct{}

func (clockImplementation) Now() time.Time                  { return time.Now() }
func (clockImplementation) Since(t time.Time) time.Duration { return time.Since(t) }

type serverTiming struct {
	w         http.ResponseWriter
	startedAt time.Time
	name      string
}

var clock Clock = clockImplementation{}

func newServerTiming(w http.ResponseWriter, name string) *serverTiming {
	return &serverTiming{w, clock.Now(), name}
}

func (t *serverTiming) End() {
	t.w.Header().Add("Server-Timing", fmt.Sprintf("%s;dur=%f", t.name, clock.Since(t.startedAt).Seconds()))
}
