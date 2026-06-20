package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ardasevinc/cx/internal/indexer"
)

func TestPrintStatusTextHumanizesCacheSize(t *testing.T) {
	var out bytes.Buffer
	printStatusText(&out, indexer.Status{
		FTSAvailable:      true,
		TotalSessions:     1062,
		IndexedSessions:   869,
		TruncatedSessions: 193,
		FreshSessions:     860,
		StaleSessions:     2,
		UncachedSessions:  7,
		ChunkCount:        76810,
		CacheBytes:        139804672,
	})

	text := out.String()
	if strings.Contains(text, "cache bytes:") || strings.Contains(text, "139804672") {
		t.Fatalf("expected human cache size, got:\n%s", text)
	}
	if !strings.Contains(text, "cache size:  133.3 MiB") {
		t.Fatalf("expected MiB cache size, got:\n%s", text)
	}
	if !strings.Contains(text, "1,062 total") || !strings.Contains(text, "76,810") {
		t.Fatalf("expected grouped counts, got:\n%s", text)
	}
	if !strings.Contains(text, "fts:         enabled") {
		t.Fatalf("expected readable fts state, got:\n%s", text)
	}
	if !strings.Contains(text, "freshness:   860 fresh, 2 stale, 7 uncached") {
		t.Fatalf("expected readable freshness state, got:\n%s", text)
	}
}
