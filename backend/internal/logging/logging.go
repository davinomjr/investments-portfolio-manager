// Package logging routes the standard library logger to stdout as one JSON
// object per line with a `level` field. Railway treats anything on stderr as
// severity "error", which made every routine line ("backend listening…")
// look like a crash; with structured stdout logs Railway shows the real level.
package logging

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// Setup points the default logger at stdout in structured form.
func Setup() {
	log.SetFlags(0) // Railway adds its own timestamps
	log.SetOutput(NewWriter(os.Stdout))
}

// Writer turns each log line into {"level": ..., "message": ...}.
type Writer struct {
	mu  sync.Mutex
	out io.Writer
}

func NewWriter(out io.Writer) *Writer { return &Writer{out: out} }

func (w *Writer) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	line, err := json.Marshal(struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}{Level: Level(msg), Message: msg})
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.out.Write(append(line, '\n')); err != nil {
		return 0, err
	}
	return len(p), nil
}

// warnMarkers flag recoverable problems where a fallback kicked in.
var warnMarkers = []string{"trying ", "falling back", "skipping", "disabled", "not found", "unexpected status", "no data for", "no parsable data"}

// errorMarkers flag real failures.
var errorMarkers = []string{"failed", "fatal", "panic", "error", "refusing"}

// Level classifies a log message by keywords.
func Level(msg string) string {
	lower := strings.ToLower(msg)
	for _, m := range warnMarkers {
		if strings.Contains(lower, m) {
			return "warn"
		}
	}
	for _, m := range errorMarkers {
		if strings.Contains(lower, m) {
			return "error"
		}
	}
	return "info"
}
