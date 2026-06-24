package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"refleks/internal/models"
)

// ComputeRunID derives a stable identity from persisted run metadata without changing the .refleks file format.
func ComputeRunID(fileName string, stats map[string]any) string {
	parts := []string{
		filepath.Base(strings.TrimSpace(fileName)),
		statString(stats, "Scenario"),
		statString(stats, "Hash"),
		statString(stats, "Date Played"),
		statString(stats, "Score"),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// EnsureRunID returns a copy of the run with RunID populated when missing.
func EnsureRunID(rec models.RunRecord) models.RunRecord {
	if strings.TrimSpace(rec.RunID) == "" {
		rec.RunID = ComputeRunID(rec.FileName, rec.Stats)
	}
	return rec
}

func statString(stats map[string]any, key string) string {
	if stats == nil {
		return ""
	}
	switch v := stats[key].(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
