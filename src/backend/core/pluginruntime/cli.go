// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

// CLICommandRow is one declared command flattened for storage. The nested
// fields travel as JSON text because the CLI reads them back whole and never
// queries inside them.
type CLICommandRow struct {
	Path        string
	Kind        string
	Method      string
	Route       string
	ArgsJSON    string
	FlagsJSON   string
	ColumnsJSON string
	Body        string
	Needs       string
	Risk        string
	GuestKind   string
	Short       string
	Long        string
	Example     string
	EnvScoped   bool
}

// cliRootNameRE is the shape a plugin's root command name must take. Lowercase
// and hyphenated so it cannot be mistaken for a flag or need quoting in a shell.
var cliRootNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// cliRegistry collects a plugin's declared commands, validates them, and
// records them.
//
// A rejection drops the whole command set and never disables the plugin: its
// routes, pages, widgets and nodes are untouched, because a bad command
// declaration must not take a working plugin offline.
type cliRegistry struct {
	runtime *Runtime
	origin  string
	root    *plugin.CLIRoot
	// paths guards uniqueness within this plugin before the unique index does,
	// so the reason a command was refused is a sentence rather than a
	// constraint violation.
	paths map[string]bool
	err   error
}

// Root claims the plugin's one top-level command group.
func (r *cliRegistry) Root(root plugin.CLIRoot) error {
	if r.err != nil {
		return r.err
	}
	name := strings.ToLower(strings.TrimSpace(root.Name))
	if !cliRootNameRE.MatchString(name) {
		return r.fail(fmt.Errorf("root name %q must match %s", root.Name, cliRootNameRE))
	}
	if reservedCLINames[name] {
		return r.fail(fmt.Errorf("root name %q is a core command", name))
	}
	for _, alias := range root.Aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if reservedCLINames[alias] {
			return r.fail(fmt.Errorf("root alias %q is a core command", alias))
		}
	}
	if owner, taken := r.runtime.cliRootOwner(name); taken && owner != r.origin {
		return r.fail(fmt.Errorf("root name %q already belongs to %s", name, owner))
	}
	root.Name = name
	r.root = &root
	r.runtime.claimCLIRoot(name, r.origin)

	if r.runtime.tracker == nil {
		return nil
	}
	aliases, err := json.Marshal(root.Aliases)
	if err != nil {
		return r.fail(fmt.Errorf("encoding aliases: %w", err))
	}
	if err := r.runtime.tracker.RecordCLIRoot(context.Background(), r.origin, name, root.Short, string(aliases)); err != nil {
		return r.fail(fmt.Errorf("recording root %q: %w", name, err))
	}
	return nil
}

// Add validates and records one command.
func (r *cliRegistry) Add(cmd plugin.CLICommand) error {
	if r.err != nil {
		return r.err
	}
	if r.root == nil {
		return r.fail(fmt.Errorf("cli registry: Root must be called before Add"))
	}
	if len(cmd.Path) == 0 {
		return r.fail(fmt.Errorf("command with route %s %s has an empty path", cmd.Method, cmd.Route))
	}
	path := strings.Join(cmd.Path, " ")
	if r.paths[path] {
		return r.fail(fmt.Errorf("duplicate command path %q", path))
	}

	if err := validateCLIKind(cmd); err != nil {
		return r.fail(fmt.Errorf("command %q: %w", path, err))
	}
	// The permission agreement. A command may never advertise a weaker key than
	// its route enforces, and may never name a route the plugin did not
	// register — which is also what makes step 0's ungated routes a
	// prerequisite rather than a detail: a route with no declared permission
	// cannot satisfy this.
	if cmd.Kind != plugin.CLIAttach {
		declared, found := r.runtime.declaredRoutePermission(r.origin, cmd.Method, cmd.Route)
		if !found {
			return r.fail(fmt.Errorf("command %q names route %s %s, which this plugin does not declare",
				path, cmd.Method, cmd.Route))
		}
		if declared != cmd.Needs {
			return r.fail(fmt.Errorf("command %q needs %q but route %s %s enforces %q",
				path, cmd.Needs, cmd.Method, cmd.Route, declared))
		}
	}

	r.paths[path] = true
	if r.runtime.tracker == nil {
		return nil
	}
	row, err := cliRow(path, cmd)
	if err != nil {
		return r.fail(fmt.Errorf("command %q: %w", path, err))
	}
	if err := r.runtime.tracker.RecordCLICommand(context.Background(), r.origin, row); err != nil {
		return r.fail(fmt.Errorf("recording command %q: %w", path, err))
	}
	return nil
}

// fail records the first rejection and returns it, so a plugin that ignores the
// error from one Add does not go on to record a partial command set.
func (r *cliRegistry) fail(err error) error {
	if r.err == nil {
		r.err = err
	}
	return err
}

// validateCLIKind checks the requirements each interaction kind carries, which
// are the ones that would otherwise produce a command that cannot work.
func validateCLIKind(cmd plugin.CLICommand) error {
	switch cmd.Kind {
	case plugin.CLIList:
		if len(cmd.Columns) == 0 {
			return fmt.Errorf("a list command needs at least one column")
		}
	case plugin.CLIAttach:
		if cmd.GuestKind == "" {
			return fmt.Errorf("an attach command needs a guest kind")
		}
	case plugin.CLIUpload:
		files := 0
		for _, f := range cmd.Flags {
			if f.Type == plugin.FlagFile {
				files++
			}
		}
		if files != 1 {
			return fmt.Errorf("an upload command needs exactly one file flag, found %d", files)
		}
	case plugin.CLIDownload:
		if len(cmd.Columns) > 0 {
			return fmt.Errorf("a download command renders no columns")
		}
	case plugin.CLIGet, plugin.CLIAction, plugin.CLIText:
	default:
		return fmt.Errorf("unknown interaction kind %q", cmd.Kind)
	}
	return nil
}

// cliRow flattens a declaration for storage.
func cliRow(path string, cmd plugin.CLICommand) (CLICommandRow, error) {
	args, err := json.Marshal(cmd.Args)
	if err != nil {
		return CLICommandRow{}, fmt.Errorf("encoding args: %w", err)
	}
	flags, err := json.Marshal(cmd.Flags)
	if err != nil {
		return CLICommandRow{}, fmt.Errorf("encoding flags: %w", err)
	}
	columns, err := json.Marshal(cmd.Columns)
	if err != nil {
		return CLICommandRow{}, fmt.Errorf("encoding columns: %w", err)
	}
	body := cmd.Body
	if body == "" {
		body = plugin.BodyNone
	}
	risk := cmd.Risk
	if risk == "" {
		risk = plugin.RiskSafe
	}
	return CLICommandRow{
		Path: path, Kind: string(cmd.Kind), Method: cmd.Method, Route: cmd.Route,
		ArgsJSON: string(args), FlagsJSON: string(flags), ColumnsJSON: string(columns),
		Body: string(body), Needs: cmd.Needs, Risk: string(risk), GuestKind: cmd.GuestKind,
		Short: cmd.Short, Long: cmd.Long, Example: cmd.Example, EnvScoped: cmd.EnvScoped,
	}, nil
}

// cliRootOwner reports which plugin owns a root name, if any.
func (r *Runtime) cliRootOwner(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	owner, ok := r.cliRoots[name]
	return owner, ok
}

// claimCLIRoot records that origin owns name.
func (r *Runtime) claimCLIRoot(name, origin string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cliRoots == nil {
		r.cliRoots = map[string]string{}
	}
	r.cliRoots[name] = origin
}

// releaseCLIRoots gives back every root a plugin claimed. Called when its
// command set is refused, so the name is free for a corrected build.
func (r *Runtime) releaseCLIRoots(origin string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, owner := range r.cliRoots {
		if owner == origin {
			delete(r.cliRoots, name)
		}
	}
}

// recordCLIError stores why a plugin's command set was refused.
func (r *Runtime) recordCLIError(origin, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cliErrors == nil {
		r.cliErrors = map[string]string{}
	}
	r.cliErrors[origin] = reason
}

// CLIError returns why a plugin's command set was refused, or an empty string
// when it was accepted. The loader status endpoint reports it, because a refused
// set leaves a working plugin with no CLI surface and no other trace.
func (r *Runtime) CLIError(origin string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cliErrors[origin]
}

// declaredRoutePermission returns the permission a plugin's manifest declares
// for one route, matched exactly as routeRegistry.lookupRouteDecl matches it at
// mount time — case-insensitive on the method, exact on the path — so a command
// cannot agree with a declaration the middleware would not.
func (r *Runtime) declaredRoutePermission(origin, method, path string) (string, bool) {
	r.mu.RLock()
	manifest, ok := r.manifests[origin]
	r.mu.RUnlock()
	if !ok {
		return "", false
	}
	for _, rt := range manifest.Contribs.Routes {
		if strings.EqualFold(rt.Method, method) && rt.Path == path {
			return rt.Requires, true
		}
	}
	return "", false
}
