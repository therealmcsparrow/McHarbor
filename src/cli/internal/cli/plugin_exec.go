// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// pluginExecutor turns one declared command into a request. Every kind reuses
// the machinery the core commands use — the same confirmation rules, the same
// renderer, the same exit codes — so a plugin command is guarded exactly as a
// core one is rather than by the plugin's own judgement.
type pluginExecutor struct {
	g   *globals
	cmd pluginCLICommand

	// values holds each flag's parsed destination, keyed by flag name.
	values map[string]any
	// flags is the command's own flag set, so an unset flag can be told from one
	// set to its zero value. Without that, an unset --vcpus writes `"vcpus": 0`
	// over the value in --data on a BodyBoth command.
	flags *pflag.FlagSet
	// outFile is the destination for a download, and dataFile the source for a
	// body read from disk.
	outFile  string
	force    bool
	dataFile string
	console  string
	// listen is where a tunnelled (vnc) console accepts a native client.
	listen string
}

// newPluginExecutor prepares the executor for one command.
func newPluginExecutor(g *globals, cmd pluginCLICommand) *pluginExecutor {
	return &pluginExecutor{g: g, cmd: cmd, values: map[string]any{}}
}

// bind declares the command's flags on the cobra command.
func (e *pluginExecutor) bind(cmd *cobra.Command) {
	fs := cmd.Flags()
	e.flags = fs
	for _, f := range e.cmd.Flags {
		if f.Name == "" || reservedFlagNames[f.Name] {
			// A declared flag that would collide with a persistent one would
			// panic cobra at startup, which is a plugin taking the whole CLI
			// down. Skipping it loses one flag instead.
			continue
		}
		switch f.Type {
		case "int":
			def := valueOr(strconv.Atoi(f.Default))
			e.values[f.Name] = fs.Int(f.Name, def, f.Help)
		case "bool":
			e.values[f.Name] = fs.Bool(f.Name, f.Default == "true", f.Help)
		case "duration":
			def := valueOr(time.ParseDuration(f.Default))
			e.values[f.Name] = fs.Duration(f.Name, def, f.Help)
		case "stringSlice":
			e.values[f.Name] = fs.StringSlice(f.Name, nil, f.Help)
		default:
			e.values[f.Name] = fs.String(f.Name, f.Default, f.Help)
		}
		if f.Required {
			warnErr(cmd.MarkFlagRequired(f.Name))
		}
	}
	switch e.cmd.Kind {
	case "download":
		fs.StringVarP(&e.outFile, "output-file", "f", "", "file to write; - for stdout")
		fs.BoolVar(&e.force, "force", false, "overwrite the destination file if it exists")
	case "attach":
		def, help := attachConsoleFlag(e.cmd.Consoles)
		fs.StringVar(&e.console, "console", def, help)
		fs.StringVar(&e.listen, "listen", defaultTunnelListen, tunnelHelp())
	}
	if e.cmd.Body == "file" || e.cmd.Body == "both" {
		fs.StringVar(&e.dataFile, "data", "", "read the request body from this file, or - for stdin")
	}
}

// reservedFlagNames are the CLI's persistent flags. A declared flag with one of
// these names panics cobra at startup rather than failing one command.
var reservedFlagNames = map[string]bool{
	"context": true, "url": true, "env": true, "ca-file": true, "insecure": true,
	"output": true, "json": true, "yaml": true, "yes": true, "confirm": true,
	"timeout": true, "help": true, "data": true, "output-file": true, "force": true,
	"console": true, "listen": true,
}

// run dispatches on the declared kind.
func (e *pluginExecutor) run(cmd *cobra.Command, args []string) error {
	op := "plugin:" + e.cmd.Slug + ":" + strings.Join(e.cmd.Path, ".")
	path, err := e.resolvePath(args)
	if err != nil {
		return err
	}
	query, body, err := e.assemble()
	if err != nil {
		return err
	}

	switch e.cmd.Kind {
	case "list":
		return e.runListNamed(cmd, op, path, query, body)
	case "get":
		return e.g.runGet(cmd, op, e.call(path, query, body), toColumns(e.cmd.Columns))
	case "action":
		target := actionTarget(args, path)
		return e.g.runAction(cmd, op, toRisk(e.cmd.Risk), e.actionVerb(), target, e.call(path, query, body))
	case "text":
		return e.streamText(cmd, op, path, query)
	case "download":
		return e.download(cmd, op, path, query)
	case "upload":
		return e.upload(cmd, op, path, query)
	case "attach":
		return e.attach(cmd, args)
	}
	return usageError("this CLI cannot run a %q command; upgrade it", e.cmd.Kind)
}

// runListNamed renders a list, telling the renderer what the rows are likely
// called. A response that carries two arrays cannot be guessed from its shape,
// and the command's own path names the one the operator asked for.
func (e *pluginExecutor) runListNamed(cmd *cobra.Command, op, path string, query url.Values, body []byte) error {
	p, err := e.g.printer()
	if err != nil {
		return err
	}
	v, err := e.g.fetch(cmd, op, e.call(path, query, body))
	if err != nil {
		return err
	}
	return p.List(listItemsPreferring(v, e.rowKeyHints()...), toColumns(e.cmd.Columns))
}

// rowKeyHints guesses what the rows are called from the command's path: the
// segment before the final verb, and its plural. `snapshot schedule list` yields
// "schedule" and "schedules".
func (e *pluginExecutor) rowKeyHints() []string {
	if len(e.cmd.Path) < 2 {
		return nil
	}
	noun := e.cmd.Path[len(e.cmd.Path)-2]
	if noun == "" {
		return nil
	}
	hints := []string{noun + "s", noun}
	if strings.HasSuffix(noun, "y") {
		hints = append(hints, strings.TrimSuffix(noun, "y")+"ies")
	}
	return hints
}

// actionVerb is what the confirmation prompt and the past-tense report say. The
// last path segment is the verb by construction: `vm snapshot delete`.
func (e *pluginExecutor) actionVerb() string {
	return e.cmd.Path[len(e.cmd.Path)-1]
}

// actionTarget is what an irreversible command's --confirm must name. The first
// positional argument is the object; with none, the route itself is the target,
// which keeps the confirmation meaningful for a whole-environment action.
func actionTarget(args []string, path string) string {
	if len(args) > 0 {
		return args[0]
	}
	return path
}

// resolvePath fills the route's placeholders from the positional arguments and
// from any path-target flags.
func (e *pluginExecutor) resolvePath(args []string) (string, error) {
	path := e.cmd.Route
	for i, name := range e.cmd.Args {
		if i >= len(args) {
			return "", usageError("missing argument <%s>", name)
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(args[i]))
	}
	for _, f := range e.cmd.Flags {
		if f.Target != "path" {
			continue
		}
		if value, ok := e.stringValue(f.Name); ok && value != "" {
			path = strings.ReplaceAll(path, "{"+f.Field+"}", url.PathEscape(value))
		}
	}
	if i := strings.Index(path, "{"); i >= 0 {
		return "", usageError("the route %s still has an unfilled placeholder at %s", e.cmd.Route, path[i:])
	}
	return path, nil
}

// assemble builds the query string and the request body from the flags.
func (e *pluginExecutor) assemble() (url.Values, []byte, error) {
	query := url.Values{}
	fields := map[string]any{}

	for _, f := range e.cmd.Flags {
		value, present := e.values[f.Name]
		if !present || !e.wasSet(f) {
			continue
		}
		switch f.Target {
		case "query":
			if rendered, ok := renderFlag(value); ok {
				query.Set(f.Field, rendered)
			}
		case "body":
			if raw, ok := flagValue(value); ok {
				setJSONPath(fields, f.Field, raw)
			}
		}
	}
	if e.cmd.EnvScoped {
		a, err := e.g.api()
		if err != nil {
			return nil, nil, err
		}
		if env := a.env(); env != nil {
			query.Set("env", *env)
		}
	}

	switch e.cmd.Body {
	case "none", "":
		return query, nil, nil
	case "flags":
		raw, err := json.Marshal(fields)
		if err != nil {
			return nil, nil, fmt.Errorf("encoding request: %w", err)
		}
		return query, raw, nil
	case "file", "both":
		base := map[string]any{}
		if e.dataFile != "" {
			raw, err := readBodyFile(e.dataFile)
			if err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(raw, &base); err != nil {
				return nil, nil, usageError("%s is not a JSON object: %v", e.dataFile, err)
			}
		} else if e.cmd.Body == "file" {
			return nil, nil, usageError("this command needs --data <file> (or - for stdin)")
		}
		if e.cmd.Body == "both" {
			for k, v := range fields {
				base[k] = v
			}
		}
		raw, err := json.Marshal(base)
		if err != nil {
			return nil, nil, fmt.Errorf("encoding request: %w", err)
		}
		return query, raw, nil
	}
	return query, nil, nil
}

// readBodyFile reads a request body from a file, or from stdin for "-".
func readBodyFile(path string) ([]byte, error) {
	if path == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, usageError("reading the body from stdin: %v", err)
		}
		return raw, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, usageError("reading %s: %v", path, err)
	}
	return raw, nil
}

// call returns the apiCall the list, get and action executors share.
func (e *pluginExecutor) call(path string, query url.Values, body []byte) apiCall {
	return func(ctx context.Context, a *API) (*http.Response, error) {
		return a.rawRequest(ctx, e.cmd.Method, path, query, body)
	}
}

// streamText writes the response body to stdout with no parsing, which is what a
// log tail wants.
func (e *pluginExecutor) streamText(cmd *cobra.Command, op, path string, query url.Values) error {
	a, err := e.g.api()
	if err != nil {
		return err
	}
	ctx, cancel := e.g.ctx(cmd)
	defer cancel()
	resp, err := a.rawRequest(ctx, e.cmd.Method, path, query, nil)
	if err != nil {
		return transportError(op, err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	if err := failOnStatus(op, resp); err != nil {
		return err
	}
	if _, err := io.Copy(e.g.stdout, resp.Body); err != nil {
		return transportError(op, err)
	}
	return nil
}

// download streams the body to a file. It refuses a terminal, because a few
// megabytes of archive through a TTY is not a useful thing to do.
func (e *pluginExecutor) download(cmd *cobra.Command, op, path string, query url.Values) error {
	a, err := e.g.api()
	if err != nil {
		return err
	}
	ctx, cancel := e.g.ctx(cmd)
	defer cancel()
	resp, err := a.rawRequest(ctx, e.cmd.Method, path, query, nil)
	if err != nil {
		return transportError(op, err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	if err := failOnStatus(op, resp); err != nil {
		return err
	}
	dest := e.outFile
	if dest == "" {
		dest = filenameFromResponse(resp, filepath.Base(strings.TrimSuffix(path, "/")))
	}
	if dest == "-" {
		if stdoutIsTerminal() {
			return usageError("refusing to write binary output to a terminal; pass --output-file <file> or redirect stdout")
		}
		if _, err := io.Copy(e.g.stdout, resp.Body); err != nil {
			return transportError(op, err)
		}
		return nil
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if e.force {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flags, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return usageError("%s already exists; pass --force to overwrite it", dest)
		}
		return usageError("creating %s: %v", dest, err)
	}
	defer func() { warnErr(f.Close()) }()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return transportError(op, err)
	}
	emitf(e.g.stderr, "Wrote %d bytes to %s.\n", n, dest)
	return nil
}

// upload posts multipart/form-data, streaming the file rather than buffering it,
// and is exempt from --timeout because an image upload is minutes.
func (e *pluginExecutor) upload(cmd *cobra.Command, op, path string, query url.Values) error {
	var fileFlag pluginFlag
	for _, f := range e.cmd.Flags {
		if f.Type == "file" {
			fileFlag = f
			break
		}
	}
	if fileFlag.Name == "" {
		return usageError("this upload command declares no file flag")
	}
	local, _ := e.stringValue(fileFlag.Name)
	if local == "" {
		return usageError("missing --%s <file>", fileFlag.Name)
	}
	src, err := os.Open(local)
	if err != nil {
		return usageError("reading %s: %v", local, err)
	}
	defer func() { warnErr(src.Close()) }()
	info, err := src.Stat()
	if err != nil {
		return usageError("reading %s: %v", local, err)
	}

	a, err := e.g.api()
	if err != nil {
		return err
	}
	// Deliberately the command's own context rather than g.ctx: an upload is
	// minutes and --timeout defaults to 60 seconds, so applying it here would
	// abort every upload of a real image.
	ctx := cmd.Context()

	field := fileFlag.Field
	if field == "" {
		field = "file"
	}
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, err := writer.CreateFormFile(field, filepath.Base(local))
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		counted := &countingReader{r: src, total: info.Size(), out: e.g.stderr}
		if _, err := io.Copy(part, counted); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		for _, f := range e.cmd.Flags {
			if f.Target != "body" || f.Type == "file" {
				continue
			}
			if rendered, ok := e.renderedValue(f.Name); ok {
				warnErr(writer.WriteField(f.Field, rendered))
			}
		}
		if err := writer.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		warnErr(pw.Close())
	}()

	resp, err := a.rawUpload(ctx, e.cmd.Method, path, query, writer.FormDataContentType(), pr)
	if err != nil {
		return transportError(op, err)
	}
	data, err := a.do(op, resp, nil)
	if err != nil {
		return err
	}
	p, err := e.g.printer()
	if err != nil {
		return err
	}
	v, err := decodeAny(data)
	if err != nil {
		return err
	}
	if v == nil {
		p.Message("%s: uploaded", filepath.Base(local))
		return nil
	}
	return p.Object(v, nil)
}

// attach opens a guest console: a ticket POST against the core console route,
// then the WebSocket. The ticket exchange belongs to the CLI, not to the plugin,
// which is why a plugin cannot point attach at a route of its own.
func (e *pluginExecutor) attach(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return usageError("attach needs the guest id")
	}
	// Refused before the ticket is minted, not after: a console ticket is
	// single-use and 30 seconds old, so spending one to then decline is a
	// ticket the operator cannot reuse.
	tunnelled := pluginTunnelConsoles[e.console]
	if tunnelled {
		if err := checkTunnelConsole(e.cmd.Consoles, e.console); err != nil {
			return err
		}
	} else if err := checkAttachConsole(e.cmd.Consoles, e.console); err != nil {
		return err
	}
	a, err := e.g.api()
	if err != nil {
		return err
	}
	env := ""
	if v := a.env(); v != nil {
		env = *v
	}
	if env == "" {
		return usageError("a console needs an environment; pass --env or set one on the context")
	}
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	if tunnelled {
		return e.tunnel(ctx, a, env, args[0])
	}

	ticket, err := e.consoleTicket(ctx, a, env, args[0])
	if err != nil {
		return err
	}
	conn, err := a.dialConsole(ctx, ticket)
	if err != nil {
		return err
	}
	defer func() { warnErr(conn.Close()) }()
	if stdinIsTerminal() {
		restore, err := makeRawStdin()
		if err != nil {
			return err
		}
		defer restore()
	}
	emitln(e.g.stderr, "Attached. Press Ctrl-] to detach.")
	return shellSession(ctx, conn, stdin, e.g.stdout)
}

// consoleTicket mints one single-use ticket for this guest's console.
func (e *pluginExecutor) consoleTicket(ctx context.Context, a *API, env, guestID string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"envId": env, "guestKind": e.cmd.GuestKind, "guestId": guestID, "kind": e.console,
	})
	if err != nil {
		return "", fmt.Errorf("encoding the console ticket request: %w", err)
	}
	resp, err := a.rawRequest(ctx, http.MethodPost, "/api/compute/console/ticket", nil, body)
	if err != nil {
		return "", transportError("computeConsoleTicket", err)
	}
	data, err := a.do("computeConsoleTicket", resp, nil)
	if err != nil {
		return "", err
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(data, &ticket); err != nil || ticket.Ticket == "" {
		return "", &Error{Code: ExitRemote, Operation: "computeConsoleTicket",
			Message: "the server issued no console ticket"}
	}
	return ticket.Ticket, nil
}

// makeRawStdin puts the terminal in raw mode and returns the restore function,
// so a console session gets every keystroke rather than lines.
func makeRawStdin() (func(), error) {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, usageError("putting the terminal into raw mode: %v", err)
	}
	return func() { warnErr(term.Restore(fd, state)) }, nil
}

// countingReader reports upload progress on stderr, so a multi-gigabyte image
// does not look like a hang.
type countingReader struct {
	r       io.Reader
	total   int64
	read    int64
	lastPct int
	out     io.Writer
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	if c.total > 0 {
		pct := int(c.read * 100 / c.total)
		if pct != c.lastPct && pct%5 == 0 {
			c.lastPct = pct
			emitf(c.out, "uploading: %d%% (%s of %s)\n", pct,
				bytesHuman(c.read), bytesHuman(c.total))
		}
	}
	return n, err
}

// toColumns converts declared columns into the renderer's own type.
func toColumns(columns []pluginColumn) []Column {
	out := make([]Column, 0, len(columns))
	for _, c := range columns {
		col := Column{Header: c.Header, Path: c.Path}
		switch c.Format {
		case "age":
			col.Format = age
		case "bytes":
			col.Format = bytesHuman
		}
		out = append(out, col)
	}
	return out
}

// toRisk maps a declared risk onto the CLI's own confirmation levels, so a
// plugin's destructive command is guarded by the same rules a core one is.
func toRisk(risk string) Risk {
	switch risk {
	case "disruptive":
		return Disruptive
	case "irreversible":
		return Irreversible
	}
	return Reversible
}

// wasSet reports whether the operator supplied this flag, or it carries a
// declared default. A flag left alone must contribute nothing: a zero int or a
// false bool is indistinguishable from an unset one by value, and on a BodyBoth
// command an unset flag would overwrite the field the --data file supplied.
func (e *pluginExecutor) wasSet(f pluginFlag) bool {
	if e.flags == nil {
		return true
	}
	flag := e.flags.Lookup(f.Name)
	if flag == nil {
		return false
	}
	return flag.Changed || f.Default != ""
}

// stringValue reads a flag's value as a string.
func (e *pluginExecutor) stringValue(name string) (string, bool) {
	if p, ok := e.values[name].(*string); ok && p != nil {
		return *p, true
	}
	return "", false
}

// renderedValue reads any flag's value as the string a form field takes.
func (e *pluginExecutor) renderedValue(name string) (string, bool) {
	return renderFlag(e.values[name])
}

// renderFlag turns a parsed flag pointer into its query or form representation,
// reporting false when it is unset.
func renderFlag(value any) (string, bool) {
	switch v := value.(type) {
	case *string:
		if v == nil || *v == "" {
			return "", false
		}
		return *v, true
	case *int:
		if v == nil {
			return "", false
		}
		return strconv.Itoa(*v), true
	case *bool:
		if v == nil || !*v {
			return "", false
		}
		return "true", true
	case *time.Duration:
		if v == nil || *v == 0 {
			return "", false
		}
		return v.String(), true
	case *[]string:
		if v == nil || len(*v) == 0 {
			return "", false
		}
		return strings.Join(*v, ","), true
	}
	return "", false
}

// flagValue turns a parsed flag pointer into the Go value a JSON body carries,
// keeping an int an int rather than a string.
func flagValue(value any) (any, bool) {
	switch v := value.(type) {
	case *string:
		if v == nil || *v == "" {
			return nil, false
		}
		return *v, true
	case *int:
		if v == nil {
			return nil, false
		}
		return *v, true
	case *bool:
		if v == nil {
			return nil, false
		}
		return *v, true
	case *time.Duration:
		if v == nil || *v == 0 {
			return nil, false
		}
		return v.String(), true
	case *[]string:
		if v == nil || len(*v) == 0 {
			return nil, false
		}
		return *v, true
	}
	return nil, false
}

// setJSONPath writes value at a dotted field path, creating the intermediate
// objects, so a flag can set `spec.memory.mb` without the plugin shipping a
// body template.
func setJSONPath(root map[string]any, field string, value any) {
	parts := strings.Split(field, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

// failOnStatus turns a >=400 response into the CLI's own error, reading the
// body so the server's message and code survive.
func failOnStatus(op string, resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return transportError(op, err)
	}
	return apiError(op, resp, body)
}

// attachConsoleFlag is the --console default and help for the consoles the plugin's
// console-subject declares. The default is the first terminal one it lists, because a
// graphical console cannot be driven from a terminal at all.
func attachConsoleFlag(declared []string) (string, string) {
	def := ""
	for _, kind := range declared {
		if pluginTerminalConsoles[kind] {
			def = kind
			break
		}
	}
	if len(declared) == 0 {
		return def, "console kind; the server declared none for this guest kind"
	}
	return def, "console kind: " + strings.Join(declared, ", ")
}

// checkAttachConsole refuses a console kind the guest kind does not declare, or one that
// is not a terminal. Each message names the declared kinds, never a guessed set.
func checkAttachConsole(declared []string, kind string) error {
	terminals := make([]string, 0, len(declared))
	for _, k := range declared {
		if pluginTerminalConsoles[k] {
			terminals = append(terminals, k)
		}
	}
	if len(terminals) == 0 {
		return usageError("this guest kind declares no terminal console (declared: %s); open it in the interface", consoleList(declared))
	}
	if !slices.Contains(declared, kind) {
		return usageError("--console must be one of %s", strings.Join(terminals, ", "))
	}
	if !pluginTerminalConsoles[kind] {
		return usageError("a %s console is not a terminal; open it in the interface, or pass --console %s", kind, strings.Join(terminals, " or "))
	}
	return nil
}

// consoleList renders a declared console list for a message, saying so when it is empty.
func consoleList(declared []string) string {
	if len(declared) == 0 {
		return "none"
	}
	return strings.Join(declared, ", ")
}
