// Package lineprotocol builds and serializes InfluxDB line protocol records.
//
// Testkube ingests numeric fields from .lp / .influx artifacts into Insights,
// deriving a metric key of {measurement}_{field}. This package only guarantees
// valid, deterministic line protocol; key normalization happens on the Testkube
// side. See https://docs.testkube.io/articles/custom-metrics-in-insights.
package lineprotocol

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type kind int

const (
	kindFloat kind = iota
	kindInt
	kindUint
)

// Value is a single numeric field value. Only numeric fields are supported
// because Testkube skips string and boolean fields during ingestion.
type Value struct {
	kind kind
	f    float64
	i    int64
	u    uint64
}

// Float returns a floating-point field value.
func Float(f float64) Value { return Value{kind: kindFloat, f: f} }

// Int returns an integer field value (serialized with the "i" suffix).
func Int(i int64) Value { return Value{kind: kindInt, i: i} }

// Uint returns an unsigned field value (serialized with the "u" suffix).
func Uint(u uint64) Value { return Value{kind: kindUint, u: u} }

// Rounded returns a float field value rounded to 6 decimal places. Use it for
// computed values (means, rates) to avoid floating-point noise like
// 0.8500000000000001 in the output; source values should use Float directly.
func Rounded(f float64) Value { return Float(math.Round(f*1e6) / 1e6) }

// AsFloat returns the value as a float64 regardless of its underlying kind.
// Used when merging colliding field keys.
func (v Value) AsFloat() float64 {
	switch v.kind {
	case kindInt:
		return float64(v.i)
	case kindUint:
		return float64(v.u)
	default:
		return v.f
	}
}

func (v Value) format() string {
	switch v.kind {
	case kindInt:
		return strconv.FormatInt(v.i, 10) + "i"
	case kindUint:
		return strconv.FormatUint(v.u, 10) + "u"
	default:
		// 'f' with -1 precision avoids scientific notation, which some line
		// protocol parsers reject.
		return strconv.FormatFloat(v.f, 'f', -1, 64)
	}
}

// Record is one measurement with its identity tags and numeric fields.
type Record struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]Value
}

// Serialize renders records as line protocol. Records sharing the same identity
// (measurement + tag set) are merged into a single line; colliding field keys
// are summed, mirroring how Testkube aggregates duplicate series in one file.
// Output lines are sorted so the result is deterministic (line order is
// insignificant in line protocol).
func Serialize(records []Record) string {
	merged := map[string]*Record{}
	for _, rec := range records {
		if len(rec.Fields) == 0 {
			continue
		}
		key := identity(rec)
		m, ok := merged[key]
		if !ok {
			clone := Record{
				Measurement: rec.Measurement,
				Tags:        copyTags(rec.Tags),
				Fields:      map[string]Value{},
			}
			for fk, fv := range rec.Fields {
				clone.Fields[fk] = fv
			}
			merged[key] = &clone
			continue
		}
		for fk, fv := range rec.Fields {
			if existing, ok := m.Fields[fk]; ok {
				m.Fields[fk] = Float(existing.AsFloat() + fv.AsFloat())
			} else {
				m.Fields[fk] = fv
			}
		}
	}

	lines := make([]string, 0, len(merged))
	for _, rec := range merged {
		lines = append(lines, format(*rec))
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func identity(rec Record) string {
	var b strings.Builder
	b.WriteString(rec.Measurement)
	keys := sortedKeys(rec.Tags)
	for _, k := range keys {
		b.WriteByte('\x00')
		b.WriteString(k)
		b.WriteByte('\x01')
		b.WriteString(rec.Tags[k])
	}
	return b.String()
}

func format(rec Record) string {
	var b strings.Builder
	b.WriteString(escapeMeasurement(rec.Measurement))
	for _, k := range sortedKeys(rec.Tags) {
		v := rec.Tags[k]
		if k == "" || v == "" {
			continue
		}
		b.WriteByte(',')
		b.WriteString(escapeTag(k))
		b.WriteByte('=')
		b.WriteString(escapeTag(v))
	}
	b.WriteByte(' ')
	fieldKeys := make([]string, 0, len(rec.Fields))
	for k := range rec.Fields {
		fieldKeys = append(fieldKeys, k)
	}
	sort.Strings(fieldKeys)
	for i, k := range fieldKeys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(escapeTag(k))
		b.WriteByte('=')
		b.WriteString(rec.Fields[k].format())
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func copyTags(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

var (
	tagEscaper         = strings.NewReplacer(",", `\,`, "=", `\=`, " ", `\ `)
	measurementEscaper = strings.NewReplacer(",", `\,`, " ", `\ `)
)

func escapeTag(s string) string         { return tagEscaper.Replace(s) }
func escapeMeasurement(s string) string { return measurementEscaper.Replace(s) }
