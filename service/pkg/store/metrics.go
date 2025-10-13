package store

import (
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

// PebbleMetrics is a compact view of metrics needed by the ingest monitor.
type PebbleMetrics struct {
	WALBytes          uint64
	WALFsyncP99Ms     float64
	L0Files           int
	L0Bytes           uint64
	CompactionBacklog uint64
}

// GetPebbleMetrics returns best-effort metrics about the pebble DB.
// Currently it computes the on-disk size of the DB directory as WALBytes
// proxy and leaves other fields zero when not available. This is a
// pragmatic starting point; replace with pebble.Metrics-derived values
// where available.
func GetPebbleMetrics() PebbleMetrics {
	var m PebbleMetrics
	if db == nil {
		return m
	}
	// best-effort: compute total bytes under dbPath
	if dbPath == "" {
		return m
	}
	var total uint64
	_ = filepath.WalkDir(dbPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		total += uint64(fi.Size())
		return nil
	})
	m.WALBytes = total
	// Try to enrich metrics by reading pebble.Metrics() directly and reflectively
	if db != nil {
		if metrics := db.Metrics(); metrics != nil {
			flat := make(map[string]float64)
			flattenStruct("", reflect.ValueOf(metrics), flat)
			if v := findMetric(flat, `(?i)wal.*(size|bytes|total)`); v > 0 {
				m.WALBytes = uint64(v)
			}
			if v := findMetric(flat, `(?i)l0.*files|(?i)level0.*files`); v > 0 {
				m.L0Files = int(v)
			}
			if v := findMetric(flat, `(?i)l0.*bytes|(?i)level0.*bytes`); v > 0 {
				m.L0Bytes = uint64(v)
			}
			if v := findMetric(flat, `(?i)fsync.*p99|(?i)wal.*fsync.*p99`); v > 0 {
				m.WALFsyncP99Ms = v
			}
			if v := findMetric(flat, `(?i)compaction.*backlog|(?i)compaction.*pending.*bytes`); v > 0 {
				m.CompactionBacklog = uint64(v)
			}
		}
	}
	return m
}

func flattenMap(prefix string, in map[string]interface{}, out map[string]float64) {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch t := v.(type) {
		case float64:
			out[key] = t
		case int:
			out[key] = float64(t)
		case int64:
			out[key] = float64(t)
		case map[string]interface{}:
			flattenMap(key, t, out)
		case []interface{}:
			// skip
		default:
			// try parse numeric from string
			if s, ok := v.(string); ok {
				// strip non-digits
				re := regexp.MustCompile(`[0-9]+(\.[0-9]+)?`)
				if m := re.FindString(s); m != "" {
					// parse
					// ignore parse errors
				}
			}
		}
	}
}

func findMetric(flat map[string]float64, pattern string) float64 {
	re := regexp.MustCompile(pattern)
	for k, v := range flat {
		if re.MatchString(k) {
			return v
		}
		// also check key tokens
		if re.MatchString(strings.ReplaceAll(k, ".", "_")) {
			return v
		}
	}
	return 0
}

// flattenStruct walks a reflect.Value of a struct or pointer and fills
// out with numeric fields keyed by dotted path.
func flattenStruct(prefix string, v reflect.Value, out map[string]float64) {
	if !v.IsValid() {
		return
	}
	// unwrap pointers
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		name := t.Field(i).Name
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		// unwrap interface pointer fields
		fv := f
		for fv.Kind() == reflect.Interface {
			if fv.IsNil() {
				fv = reflect.Value{}
				break
			}
			fv = fv.Elem()
		}
		switch fv.Kind() {
		case reflect.Struct:
			flattenStruct(key, fv, out)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out[key] = float64(fv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out[key] = float64(fv.Uint())
		case reflect.Float32, reflect.Float64:
			out[key] = fv.Float()
		default:
			// ignore other kinds
		}
	}
}
