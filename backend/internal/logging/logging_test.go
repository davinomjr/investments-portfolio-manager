package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"
)

func TestLevel(t *testing.T) {
	cases := map[string]string{
		"backend listening on http://0.0.0.0:8080":                           "info",
		"sync scheduler: 08:00,22:00 BRT for [pluggy, ibkr]":                 "info",
		"td refresh complete in 53.5s (130 products)":                        "info",
		"quote fan-out: 20 requests, 18 resolved, 1.2s":                      "info",
		"usd-brl awesomeapi failed: status 429; trying frankfurter":          "warn",
		"fundsexplorer fii: no data for XPML11, falling back to Fundamentus": "warn",
		"sync scheduler: pluggy already running (job 3), skipping":           "warn",
		"quote fetch failed for XUSE: yahoo http 404":                        "error",
		"sync scheduler: scheduled ibkr sync failed: boom":                   "error",
		"startup failed: database is locked":                                 "error",
	}
	for msg, want := range cases {
		if got := Level(msg); got != want {
			t.Errorf("Level(%q)=%q want %q", msg, got, want)
		}
	}
}

func TestWriterEmitsJSONLines(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(NewWriter(&buf), "", 0)
	logger.Printf("backend listening on %s", "0.0.0.0:8080")
	logger.Printf("quote fetch failed for %s", "XUSE")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", buf.String())
	}
	var first, second struct{ Level, Message string }
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.Level != "info" || first.Message != "backend listening on 0.0.0.0:8080" {
		t.Errorf("first: %+v", first)
	}
	if second.Level != "error" {
		t.Errorf("second: %+v", second)
	}
}
