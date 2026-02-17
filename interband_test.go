package interband

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSafeKey(t *testing.T) {
	got := SafeKey("a/b c?d")
	if got != "a_b_c_d" {
		t.Fatalf("unexpected safe key: %q", got)
	}
}

func TestPathAndWriteReadKnownPayload(t *testing.T) {
	t.Setenv("INTERBAND_ROOT", t.TempDir())

	p, err := Path("interphase", "bead", "session 1")
	if err != nil {
		t.Fatalf("path error: %v", err)
	}

	payload := map[string]any{
		"id":     "iv-hoqj",
		"phase":  "executing",
		"reason": "testing",
		"ts":     123.0,
	}
	if err := Write(p, "interphase", "bead_phase", "sess1", payload); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	env, err := ReadEnvelope(p)
	if err != nil {
		t.Fatalf("read envelope failed: %v", err)
	}
	if env.Namespace != "interphase" || env.Type != "bead_phase" {
		t.Fatalf("unexpected envelope: %+v", env)
	}

	out, err := ReadPayload(p)
	if err != nil {
		t.Fatalf("read payload failed: %v", err)
	}
	if out["id"] != "iv-hoqj" {
		t.Fatalf("unexpected payload id: %#v", out["id"])
	}
}

func TestWriteRejectsInvalidKnownPayload(t *testing.T) {
	t.Setenv("INTERBAND_ROOT", t.TempDir())
	p, err := Path("interphase", "bead", "bad")
	if err != nil {
		t.Fatalf("path error: %v", err)
	}

	err = Write(p, "interphase", "bead_phase", "sess", map[string]any{
		"id":     "iv-hoqj",
		"phase":  "not-a-phase",
		"reason": "bad",
		"ts":     1,
	})
	if err == nil {
		t.Fatal("expected validation error for invalid phase")
	}
}

func TestUnknownPayloadAcceptedAsObject(t *testing.T) {
	t.Setenv("INTERBAND_ROOT", t.TempDir())
	p, err := Path("custom", "events", "x")
	if err != nil {
		t.Fatalf("path error: %v", err)
	}
	if err := Write(p, "custom", "anything", "sess", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("write failed for unknown payload: %v", err)
	}
}

func TestReadRejectsInvalidEnvelopeVersion(t *testing.T) {
	t.Setenv("INTERBAND_ROOT", t.TempDir())
	p, err := Path("custom", "events", "v2")
	if err != nil {
		t.Fatalf("path error: %v", err)
	}

	content := map[string]any{
		"version":    "2.0.0",
		"namespace":  "custom",
		"type":       "anything",
		"session_id": "s",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload":    map[string]any{"k": "v"},
	}
	raw, _ := json.Marshal(content)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	if _, err := ReadPayload(p); err == nil {
		t.Fatal("expected read to fail for unsupported version")
	}
}

func TestPruneChannelRetention(t *testing.T) {
	root := t.TempDir()
	t.Setenv("INTERBAND_ROOT", root)
	t.Setenv("INTERBAND_RETENTION_INTERPHASE_BEAD_SECS", "1")
	t.Setenv("INTERBAND_PRUNE_INTERVAL_SECS", "0")

	dir, err := ChannelDir("interphase", "bead")
	if err != nil {
		t.Fatalf("channel dir error: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldFile := filepath.Join(dir, "old.json")
	newFile := filepath.Join(dir, "new.json")
	if err := os.WriteFile(oldFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write old failed: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write new failed: %v", err)
	}
	oldTime := time.Now().Add(-3 * time.Second)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes failed: %v", err)
	}

	if err := PruneChannel("interphase", "bead"); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("expected old file pruned, stat err=%v", err)
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatalf("expected new file to remain: %v", err)
	}
}

func TestPruneChannelMaxFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("INTERBAND_ROOT", root)
	t.Setenv("INTERBAND_MAX_FILES_INTERLOCK_COORDINATION", "2")
	t.Setenv("INTERBAND_RETENTION_INTERLOCK_COORDINATION_SECS", "3600")
	t.Setenv("INTERBAND_PRUNE_INTERVAL_SECS", "0")

	dir, err := ChannelDir("interlock", "coordination")
	if err != nil {
		t.Fatalf("channel dir error: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	files := []string{
		filepath.Join(dir, "a.json"),
		filepath.Join(dir, "b.json"),
		filepath.Join(dir, "c.json"),
	}
	for idx, f := range files {
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		mt := time.Now().Add(time.Duration(-(3 - idx)) * time.Second)
		if err := os.Chtimes(f, mt, mt); err != nil {
			t.Fatalf("chtimes failed: %v", err)
		}
	}

	if err := PruneChannel("interlock", "coordination"); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "a.json")); !os.IsNotExist(err) {
		t.Fatalf("expected oldest file pruned, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.json")); err != nil {
		t.Fatalf("expected b.json to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "c.json")); err != nil {
		t.Fatalf("expected c.json to remain: %v", err)
	}
}
