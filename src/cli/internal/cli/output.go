// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"
)

// Column picks one field of a row by dotted path for table and TSV output.
type Column struct {
	Header string
	Path   string
	Format func(any) string
}

// Printer renders command results in the selected format. Data goes to out;
// progress and prompts never do, so a pipe only ever sees the result.
type Printer struct {
	format string
	out    io.Writer
}

var formats = []string{"table", "json", "yaml", "tsv"}

func newPrinter(format string, out io.Writer) (*Printer, error) {
	for _, f := range formats {
		if f == format {
			return &Printer{format: format, out: out}, nil
		}
	}
	return nil, usageError("unknown output format %q; use one of %s", format, strings.Join(formats, ", "))
}

// List renders rows; table and TSV use columns, JSON and YAML the full rows.
func (p *Printer) List(rows []map[string]any, cols []Column) error {
	switch p.format {
	case "json":
		return p.json(rows)
	case "yaml":
		return p.yaml(rows)
	case "tsv":
		for _, row := range rows {
			fields := make([]string, len(cols))
			for i, c := range cols {
				fields[i] = strings.ReplaceAll(cell(row, c), "\t", " ")
			}
			if _, err := fmt.Fprintln(p.out, strings.Join(fields, "\t")); err != nil {
				return err
			}
		}
		return nil
	}
	tw := tabwriter.NewWriter(p.out, 0, 0, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}
	emitln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fields := make([]string, len(cols))
		for i, c := range cols {
			fields[i] = cell(row, c)
		}
		emitln(tw, strings.Join(fields, "\t"))
	}
	return tw.Flush()
}

// Object renders one result; table shows the given fields, or every scalar
// field in key order when none are given.
func (p *Printer) Object(v any, cols []Column) error {
	switch p.format {
	case "json":
		return p.json(v)
	case "yaml":
		return p.yaml(v)
	}
	m, ok := v.(map[string]any)
	if !ok {
		_, err := fmt.Fprintln(p.out, render(v))
		return err
	}
	if len(cols) == 0 {
		keys := make([]string, 0, len(m))
		for k, val := range m {
			if isScalar(val) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			cols = append(cols, Column{Header: k, Path: k})
		}
	}
	if p.format == "tsv" {
		for _, c := range cols {
			if _, err := fmt.Fprintf(p.out, "%s\t%s\n", c.Header, cell(m, c)); err != nil {
				return err
			}
		}
		return nil
	}
	tw := tabwriter.NewWriter(p.out, 0, 0, 2, ' ', 0)
	for _, c := range cols {
		emitf(tw, "%s:\t%s\n", c.Header, cell(m, c))
	}
	return tw.Flush()
}

// Message prints a one-line result in table mode and nothing structured
// otherwise, so JSON consumers are never handed prose.
func (p *Printer) Message(format string, args ...any) {
	if p.format == "table" {
		emitf(p.out, format+"\n", args...)
	}
}

func (p *Printer) json(v any) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) yaml(v any) error {
	enc := yaml.NewEncoder(p.out)
	enc.SetIndent(2)
	if err := enc.Encode(yamlSafe(v)); err != nil {
		return err
	}
	return enc.Close()
}

// yamlSafe turns json.Number into a plain number so YAML does not quote it.
func yamlSafe(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = yamlSafe(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = yamlSafe(val)
		}
		return out
	case []map[string]any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = yamlSafe(val)
		}
		return out
	}
	return v
}

func cell(row map[string]any, c Column) string {
	v := lookup(row, c.Path)
	if c.Format != nil {
		return c.Format(v)
	}
	return render(v)
}

// lookup walks a dotted path; a numeric segment indexes an array.
func lookup(v any, path string) any {
	for _, seg := range strings.Split(path, ".") {
		switch t := v.(type) {
		case map[string]any:
			v = t[seg]
		case []any:
			var i int
			if _, err := fmt.Sscanf(seg, "%d", &i); err != nil || i < 0 || i >= len(t) {
				return nil
			}
			v = t[i]
		default:
			return nil
		}
	}
	return v
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, bool, json.Number, float64, int, int64, nil:
		return true
	}
	return false
}

func render(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case json.Number:
		return t.String()
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s := render(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	case map[string]any:
		raw, err := json.Marshal(t)
		if err != nil {
			return "{…}"
		}
		return string(raw)
	}
	return fmt.Sprint(v)
}

// shortID trims a Docker id to the 12 characters people recognise.
func shortID(v any) string {
	s := strings.TrimPrefix(render(v), "sha256:")
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// firstName renders Docker's Names array without its leading slash.
func firstName(v any) string {
	if arr, ok := v.([]any); ok && len(arr) > 0 {
		return strings.TrimPrefix(render(arr[0]), "/")
	}
	return strings.TrimPrefix(render(v), "/")
}

// age renders a Unix timestamp or RFC3339 string as a relative age.
func age(v any) string {
	var t time.Time
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil && n > 0 {
			t = time.Unix(n, 0)
		}
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, x); err == nil {
				t = parsed
				break
			}
		}
		if t.IsZero() {
			return x
		}
	}
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// bytesHuman renders a byte count in binary units.
func bytesHuman(v any) string {
	n, ok := v.(json.Number)
	if !ok {
		return render(v)
	}
	f, err := n.Float64()
	if err != nil {
		return n.String()
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
