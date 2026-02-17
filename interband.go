package interband

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Envelope is the v1 interband message wrapper.
type Envelope struct {
	Version   string         `json:"version"`
	Namespace string         `json:"namespace"`
	Type      string         `json:"type"`
	SessionID string         `json:"session_id"`
	Timestamp string         `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

var allowedPhases = map[string]struct{}{
	"brainstorm":          {},
	"brainstorm-reviewed": {},
	"strategized":         {},
	"planned":             {},
	"plan-reviewed":       {},
	"executing":           {},
	"shipping":            {},
	"done":                {},
}

func Root() string {
	if root := strings.TrimSpace(os.Getenv("INTERBAND_ROOT")); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".interband"
	}
	return filepath.Join(home, ".interband")
}

func ProtocolVersion() string {
	if ver := strings.TrimSpace(os.Getenv("INTERBAND_PROTOCOL_VERSION")); ver != "" {
		return ver
	}
	return "1.0.0"
}

func SafeKey(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == '/' || unicode.IsSpace(r):
			b.WriteByte('_')
		case (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "default"
	}
	return out
}

func ChannelDir(namespace, channel string) (string, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(channel) == "" {
		return "", errors.New("namespace and channel are required")
	}
	return filepath.Join(Root(), namespace, channel), nil
}

func Path(namespace, channel, key string) (string, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(channel) == "" || strings.TrimSpace(key) == "" {
		return "", errors.New("namespace, channel, and key are required")
	}
	return filepath.Join(Root(), namespace, channel, SafeKey(key)+".json"), nil
}

func ValidatePayload(namespace, typ string, payload map[string]any) error {
	if payload == nil {
		return errors.New("payload must be an object")
	}

	switch namespace + ":" + typ {
	case "interphase:bead_phase":
		if !isNonEmptyString(payload["id"]) {
			return errors.New("interphase/bead_phase: id must be a non-empty string")
		}
		phase, ok := payload["phase"].(string)
		if !ok || phase == "" {
			return errors.New("interphase/bead_phase: phase must be a non-empty string")
		}
		if _, ok := allowedPhases[phase]; !ok {
			return fmt.Errorf("interphase/bead_phase: unknown phase %q", phase)
		}
		if v, exists := payload["reason"]; exists && v != nil {
			if _, ok := v.(string); !ok {
				return errors.New("interphase/bead_phase: reason must be a string")
			}
		}
		if !isNumber(payload["ts"]) {
			return errors.New("interphase/bead_phase: ts must be numeric")
		}
	case "clavain:dispatch":
		for _, key := range []string{"name", "workdir", "activity"} {
			if !isNonEmptyString(payload[key]) {
				return fmt.Errorf("clavain/dispatch: %s must be a non-empty string", key)
			}
		}
		for _, key := range []string{"started", "turns", "commands", "messages"} {
			if !isNonNegativeNumber(payload[key]) {
				return fmt.Errorf("clavain/dispatch: %s must be a non-negative number", key)
			}
		}
	case "interlock:coordination_signal":
		for _, key := range []string{"layer", "icon", "text", "ts"} {
			if !isNonEmptyString(payload[key]) {
				return fmt.Errorf("interlock/coordination_signal: %s must be a non-empty string", key)
			}
		}
		if !isNonNegativeNumber(payload["priority"]) {
			return errors.New("interlock/coordination_signal: priority must be a non-negative number")
		}
	}

	return nil
}

func ValidateEnvelope(env Envelope) error {
	if !strings.HasPrefix(env.Version, "1.") {
		return fmt.Errorf("unsupported version %q", env.Version)
	}
	if strings.TrimSpace(env.Namespace) == "" {
		return errors.New("namespace is required")
	}
	if strings.TrimSpace(env.Type) == "" {
		return errors.New("type is required")
	}
	if strings.TrimSpace(env.Timestamp) == "" {
		return errors.New("timestamp is required")
	}
	if env.Payload == nil {
		return errors.New("payload must be an object")
	}
	return ValidatePayload(env.Namespace, env.Type, env.Payload)
}

func Write(targetPath, namespace, typ, sessionID string, payload map[string]any) error {
	if strings.TrimSpace(targetPath) == "" {
		return errors.New("target path is required")
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(typ) == "" {
		return errors.New("namespace and type are required")
	}
	if err := ValidatePayload(namespace, typ, payload); err != nil {
		return err
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".interband-tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	env := Envelope{
		Version:   ProtocolVersion(),
		Namespace: namespace,
		Type:      typ,
		SessionID: sessionID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}

	enc := json.NewEncoder(tmpFile)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func ReadEnvelope(sourcePath string) (Envelope, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return Envelope{}, errors.New("source path is required")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return Envelope{}, err
	}

	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, err
	}
	if err := ValidateEnvelope(env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func ReadPayload(sourcePath string) (map[string]any, error) {
	env, err := ReadEnvelope(sourcePath)
	if err != nil {
		return nil, err
	}
	return env.Payload, nil
}

func DefaultRetentionSeconds(namespace, channel string) int {
	switch namespace + ":" + channel {
	case "clavain:dispatch":
		return 21600 // 6h
	case "interlock:coordination":
		return 43200 // 12h
	case "interphase:bead":
		return 86400 // 24h
	default:
		return 86400
	}
}

func DefaultMaxFiles(namespace, channel string) int {
	switch namespace + ":" + channel {
	case "clavain:dispatch":
		return 128
	case "interlock:coordination":
		return 256
	case "interphase:bead":
		return 256
	default:
		return 256
	}
}

func RetentionSeconds(namespace, channel string) int {
	if v, ok := parseEnvInt(retentionEnvKey(namespace, channel)); ok {
		return v
	}
	if v, ok := parseEnvInt("INTERBAND_RETENTION_SECS"); ok {
		return v
	}
	return DefaultRetentionSeconds(namespace, channel)
}

func MaxFiles(namespace, channel string) int {
	if v, ok := parseEnvInt(maxFilesEnvKey(namespace, channel)); ok {
		return v
	}
	if v, ok := parseEnvInt("INTERBAND_MAX_FILES"); ok {
		return v
	}
	return DefaultMaxFiles(namespace, channel)
}

func PruneChannel(namespace, channel string) error {
	dir, err := ChannelDir(namespace, channel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	now := time.Now()
	pruneInterval := 300
	if v, ok := parseEnvInt("INTERBAND_PRUNE_INTERVAL_SECS"); ok {
		pruneInterval = v
	}
	if pruneInterval < 0 {
		pruneInterval = 0
	}

	stamp := filepath.Join(dir, ".interband-prune.stamp")
	if info, err := os.Stat(stamp); err == nil {
		if now.Sub(info.ModTime()) < time.Duration(pruneInterval)*time.Second {
			return nil
		}
	}
	_ = os.WriteFile(stamp, []byte{}, 0o644)

	retention := time.Duration(RetentionSeconds(namespace, channel)) * time.Second
	if retention < 0 {
		retention = 0
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		age := now.Sub(info.ModTime())
		if age > retention {
			_ = os.Remove(full)
			continue
		}
		files = append(files, fileInfo{path: full, modTime: info.ModTime()})
	}

	maxFiles := MaxFiles(namespace, channel)
	if maxFiles <= 0 || len(files) <= maxFiles {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path > files[j].path
		}
		return files[i].modTime.After(files[j].modTime)
	})

	for idx := maxFiles; idx < len(files); idx++ {
		_ = os.Remove(files[idx].path)
	}
	return nil
}

func isNonEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func isNonNegativeNumber(v any) bool {
	switch n := v.(type) {
	case int:
		return n >= 0
	case int8:
		return n >= 0
	case int16:
		return n >= 0
	case int32:
		return n >= 0
	case int64:
		return n >= 0
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return n >= 0
	case float64:
		return n >= 0
	case json.Number:
		f, err := n.Float64()
		return err == nil && f >= 0
	default:
		return false
	}
}

func parseEnvInt(name string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

func retentionEnvKey(namespace, channel string) string {
	return "INTERBAND_RETENTION_" + envSafe(namespace) + "_" + envSafe(channel) + "_SECS"
}

func maxFilesEnvKey(namespace, channel string) string {
	return "INTERBAND_MAX_FILES_" + envSafe(namespace) + "_" + envSafe(channel)
}

func envSafe(raw string) string {
	upper := strings.ToUpper(raw)
	var b strings.Builder
	for _, r := range upper {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
