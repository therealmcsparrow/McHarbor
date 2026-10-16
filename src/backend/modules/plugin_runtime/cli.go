// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin/contract"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/version"
)

// CLIRootEntry is one plugin's top-level command group as the CLI reads it.
type CLIRootEntry struct {
	Slug    string   `json:"plugin_slug"`
	Name    string   `json:"name"`
	Short   string   `json:"short,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// CLICommandEntry is one declared command. The nested shapes are decoded from
// their stored JSON so the CLI receives structure rather than strings inside
// strings.
type CLICommandEntry struct {
	Slug      string             `json:"plugin_slug"`
	Path      []string           `json:"path"`
	Kind      string             `json:"kind"`
	Method    string             `json:"method"`
	Route     string             `json:"route"`
	Args      []string           `json:"args,omitempty"`
	Flags     []plugin.CLIFlag   `json:"flags,omitempty"`
	Columns   []plugin.CLIColumn `json:"columns,omitempty"`
	Body      string             `json:"body"`
	Needs     string             `json:"needs,omitempty"`
	Risk      string             `json:"risk"`
	GuestKind string             `json:"guest_kind,omitempty"`
	Consoles  []string           `json:"consoles,omitempty"`
	Short     string             `json:"short,omitempty"`
	Long      string             `json:"long,omitempty"`
	Example   string             `json:"example,omitempty"`
	EnvScoped bool               `json:"env_scoped,omitempty"`
}

// CLISpec is the whole command surface one instance offers.
//
// SpecVersion lets a CLI that does not understand the shape ignore it entirely
// and say so in `doctor`, rather than guess; Build tells the operator which
// server the cached tree came from.
type CLISpec struct {
	SpecVersion int               `json:"specVersion"`
	Build       string            `json:"build"`
	Roots       []CLIRootEntry    `json:"roots"`
	Commands    []CLICommandEntry `json:"commands"`
}

// ListCLI returns every plugin-contributed command group and command.
func (s *Service) ListCLI(ctx context.Context) (CLISpec, error) {
	spec := CLISpec{
		SpecVersion: plugin.CLISpecVersion,
		Build:       version.Build(),
		Roots:       make([]CLIRootEntry, 0, 8),
		Commands:    make([]CLICommandEntry, 0, 256),
	}

	rootRows, err := s.db.QueryContext(ctx, `
		SELECT origin, name, COALESCE(short, ''), COALESCE(aliases, '[]')
		  FROM plugin_cli_roots ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return spec, fmt.Errorf("loading plugin cli roots: %w", err)
	}
	defer func() {
		if err := rootRows.Close(); err != nil {
			slog.Warn("plugin runtime: closing cli root rows", "error", err)
		}
	}()
	// rootOf maps a plugin to its root name, so a command row does not have to
	// repeat it and a command whose root was refused is dropped rather than
	// served unreachable.
	rootOf := map[string]string{}
	for rootRows.Next() {
		var entry CLIRootEntry
		var aliases string
		if err := rootRows.Scan(&entry.Slug, &entry.Name, &entry.Short, &aliases); err != nil {
			return spec, fmt.Errorf("scanning plugin cli roots: %w", err)
		}
		if err := json.Unmarshal([]byte(aliases), &entry.Aliases); err != nil {
			entry.Aliases = nil
		}
		rootOf[entry.Slug] = entry.Name
		spec.Roots = append(spec.Roots, entry)
	}
	if err := rootRows.Err(); err != nil {
		return spec, fmt.Errorf("reading plugin cli roots: %w", err)
	}

	cmdRows, err := s.db.QueryContext(ctx, `
		SELECT origin, path, kind, method, route, COALESCE(args, '[]'),
		       COALESCE(flags, '[]'), COALESCE(columns, '[]'), COALESCE(body, 'none'),
		       COALESCE(needs, ''), COALESCE(risk, 'safe'), COALESCE(guest_kind, ''),
		       COALESCE(short, ''), COALESCE(long, ''), COALESCE(example, ''),
		       COALESCE(env_scoped, 0)
		  FROM plugin_cli_commands ORDER BY origin ASC, path ASC LIMIT 1000`)
	if err != nil {
		return spec, fmt.Errorf("loading plugin cli commands: %w", err)
	}
	defer func() {
		if err := cmdRows.Close(); err != nil {
			slog.Warn("plugin runtime: closing cli command rows", "error", err)
		}
	}()
	for cmdRows.Next() {
		var (
			entry                      CLICommandEntry
			path, args, flags, columns string
			envScoped                  int
		)
		if err := cmdRows.Scan(&entry.Slug, &path, &entry.Kind, &entry.Method, &entry.Route,
			&args, &flags, &columns, &entry.Body, &entry.Needs, &entry.Risk,
			&entry.GuestKind, &entry.Short, &entry.Long, &entry.Example, &envScoped); err != nil {
			return spec, fmt.Errorf("scanning plugin cli commands: %w", err)
		}
		if _, ok := rootOf[entry.Slug]; !ok {
			continue
		}
		entry.Path = splitCLIPath(path)
		entry.EnvScoped = envScoped != 0
		if err := json.Unmarshal([]byte(args), &entry.Args); err != nil {
			entry.Args = nil
		}
		if err := json.Unmarshal([]byte(flags), &entry.Flags); err != nil {
			entry.Flags = nil
		}
		if err := json.Unmarshal([]byte(columns), &entry.Columns); err != nil {
			entry.Columns = nil
		}
		if entry.Kind == string(plugin.CLIAttach) {
			entry.Consoles = declaredConsoles(plugincaps.Default, entry.Slug, entry.GuestKind)
		}
		spec.Commands = append(spec.Commands, entry)
	}
	return spec, cmdRows.Err()
}

// declaredConsoles is the console list the plugin's own console-subject declares for
// guestKind, so the CLI never guesses which consoles a guest kind has. Another plugin's
// declaration of the same kind does not count: the attach command is this plugin's.
func declaredConsoles(reg *plugincaps.Registry, slug, guestKind string) []string {
	c, ok := reg.ConsoleSubject(guestKind)
	if !ok || c.Plugin != slug {
		return nil
	}
	return slices.Clone(c.Consoles)
}

// splitCLIPath turns the stored space-joined path back into its segments.
func splitCLIPath(path string) []string {
	return strings.Fields(path)
}

// handleCLI serves the command spec, with an ETag so the steady state of a CLI
// that revalidates on every invocation is a 304 on a request that was happening
// anyway.
func (h *Handler) handleCLI(w http.ResponseWriter, r *http.Request) {
	spec, err := h.svc.ListCLI(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading plugin cli spec", err)
		return
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encoding plugin cli spec", err)
		return
	}
	sum := sha256.Sum256(raw)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	response.OK(w, spec)
}
