// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// terminalSize reports the local terminal's size; ok is false when stdout is
// not a terminal. It is a variable so tests can drive resizes.
var terminalSize = func() (cols, rows int, ok bool) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	return w, h, err == nil
}

// makeRaw puts stdin in raw mode so keystrokes such as Ctrl-C reach the remote
// shell instead of this process, and returns how to put it back.
var makeRaw = func() (func(), error) {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() {
		if err := term.Restore(fd, state); err != nil {
			fmt.Fprintln(os.Stderr, "restoring the terminal:", err)
		}
	}, nil
}

// resizePoll is how often the session compares the terminal size. Polling works
// on every platform, where SIGWINCH does not exist on Windows.
var resizePoll = time.Second

// remoteExit is a command that ran and exited non-zero: the process exits with
// the remote status and prints nothing more, as ssh and docker exec do.
type remoteExit int

func (e remoteExit) Error() string { return fmt.Sprintf("the command exited with status %d", int(e)) }

type execOptions struct {
	shell, user, workDir string
	debug                bool
	debugSession         string
}

func execCommand(g *globals) *cobra.Command {
	var o execOptions
	cmd := &cobra.Command{
		Use:   "exec <id|name> [-- command [args...]]",
		Short: "Run a command in a container, or open an interactive shell",
		Long: "With a command after --, exec runs it once and prints its output; mcharbor then exits with the command's own status. " +
			"Without one it opens an interactive shell over the same audited session the web terminal uses, which needs a terminal. " +
			"A command needs containers.manage; a shell needs terminal.access or containers.manage. " +
			"--debug is for a distroless or scratch image with no shell: McHarbor uploads a static busybox to /.mcharbor-debug, " +
			"runs the shell or the command with it, and removes it when the session ends (at the latest 15 minutes later). " +
			"It needs containers.debug_shell, plus containers.manage with a command; every install is audited with the binary's digest.",
		Example: "  mcharbor container exec web -- cat /etc/os-release\n  mcharbor container exec web --shell /bin/bash\n" +
			"  mcharbor container exec distroless-api --debug\n  mcharbor container exec distroless-api --debug -- ls -la /app",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.debug && o.shell != "" {
				return usageError("--debug runs the uploaded busybox shell; it cannot be combined with --shell")
			}
			if o.debug {
				if cmd.ArgsLenAtDash() == 1 && len(args) > 1 {
					return g.runDebugExec(cmd, args[0], args[1:], o)
				}
				if len(args) > 1 {
					return usageError("put the command after --, as in: mcharbor container exec %s --debug -- %s", args[0], strings.Join(args[1:], " "))
				}
				return g.runDebugShell(cmd, args[0], o)
			}
			if cmd.ArgsLenAtDash() == 1 && len(args) > 1 {
				return g.runExec(cmd, args[0], args[1:], o)
			}
			if len(args) > 1 {
				return usageError("put the command after --, as in: mcharbor container exec %s -- %s", args[0], strings.Join(args[1:], " "))
			}
			return g.runShell(cmd, args[0], o)
		},
	}
	cmd.Flags().StringVar(&o.shell, "shell", "", "shell for an interactive session (default /bin/sh, or the best one found); with a command, run it through this shell with -c")
	cmd.Flags().StringVarP(&o.user, "user", "u", "", "user to run as, instead of the image's USER")
	cmd.Flags().StringVarP(&o.workDir, "workdir", "w", "", "working directory inside the container")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "for a shell-less image: upload a temporary static busybox and run with it (needs containers.debug_shell)")
	return cmd
}

// shellless recognises the container runtime's complaint that there is nothing
// in the image to exec. A distroless or scratch image carries no shell at all,
// and the raw message — "exec: \"/bin/sh\": stat /bin/sh: no such file or
// directory" — reads as a McHarbor fault rather than as a property of the image.
func shellless(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(asError(err).Error())
	if !strings.Contains(msg, "no such file or directory") && !strings.Contains(msg, "executable file not found") {
		return false
	}
	for _, needle := range []string{"/bin/sh", "/bin/bash", "/bin/ash", "/bin/busybox", "exec:", "starting container process"} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// withShelllessHint turns the runtime's stat error into one an operator can act
// on. It changes no exit code: the command still failed, and for the same reason.
func withShelllessHint(err error, target string) error {
	if !shellless(err) {
		return err
	}
	e := asError(err)
	hint := *e
	hint.Message += "; " + target + " has no shell to run — a distroless or scratch image carries none. " +
		"Run a binary the image does contain by naming it after -- , point --shell at a shell that is " +
		"actually there, or reach the filesystem from a sidecar sharing the namespace"
	return &hint
}

func (g *globals) runExec(cmd *cobra.Command, target string, command []string, o execOptions) error {
	v, err := g.fetch(cmd, "containersExec", func(ctx context.Context, a *API) (*http.Response, error) {
		body := client.ContainersExecRequest{Command: command, Shell: strPtr(o.shell), User: strPtr(o.user), WorkDir: strPtr(o.workDir)}
		return a.c.ContainersExec(ctx, target, &client.ContainersExecParams{Env: a.env()}, body)
	})
	if err != nil {
		return withShelllessHint(err, target)
	}
	p, err := g.printer()
	if err != nil {
		return err
	}
	if p.format != "table" {
		return p.Object(v, nil)
	}
	emit(g.stdout, render(lookup(v, "stdout")))
	emit(g.stderr, render(lookup(v, "stderr")))
	if n, ok := lookup(v, "exitCode").(json.Number); ok {
		if code, err := n.Int64(); err == nil && code != 0 {
			return remoteExit(int(code))
		}
	}
	return nil
}

func (g *globals) runShell(cmd *cobra.Command, target string, o execOptions) error {
	if !stdinIsTerminal() {
		return usageError("an interactive shell needs a terminal; to run one command, put it after --")
	}
	a, err := g.api()
	if err != nil {
		return err
	}
	conn, err := a.dialTerminal(cmd.Context(), target, o)
	if err != nil {
		return withShelllessHint(err, target)
	}
	defer func() { warnErr(conn.Close()) }()
	restore, err := makeRaw()
	if err != nil {
		return usageError("switching the terminal to raw mode: %v", err)
	}
	defer restore()
	// The dial succeeds even for a shell-less image: the server only discovers
	// there is nothing to exec once it tries, and reports it as an error frame.
	return withShelllessHint(shellSession(cmd.Context(), conn, stdin, g.stdout), target)
}

// dialTerminal opens the web terminal's WebSocket. The server refuses an
// upgrade with no Origin, so the CLI names the server itself, which is the
// origin a browser on the same URL would send.
func (a *API) dialTerminal(ctx context.Context, target string, o execOptions) (*websocket.Conn, error) {
	base, err := url.Parse(strings.TrimRight(a.target.URL, "/"))
	if err != nil {
		return nil, usageError("invalid server URL %q: %v", a.target.URL, err)
	}
	origin := base.Scheme + "://" + base.Host
	ws := *base
	switch base.Scheme {
	case "https":
		ws.Scheme = "wss"
	case "http":
		ws.Scheme = "ws"
	default:
		return nil, usageError("the server URL must be http or https, not %q", base.Scheme)
	}
	ws.Path = base.Path + "/api/terminal/ws"
	q := url.Values{"container": {target}}
	for k, v := range map[string]string{"shell": o.shell, "user": o.user, "workDir": o.workDir, "env": a.target.Env, "debugSession": o.debugSession} {
		if v != "" {
			q.Set(k, v)
		}
	}
	ws.RawQuery = q.Encode()

	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 30 * time.Second}
	if t, ok := a.http.Transport.(*http.Transport); ok {
		dialer.TLSClientConfig = t.TLSClientConfig
	}
	h := http.Header{"User-Agent": {"mcharbor-cli/" + Version}, "Origin": {origin}}
	if a.target.APIKey != "" {
		h.Set("Authorization", "Bearer "+a.target.APIKey)
	}
	conn, resp, err := dialer.DialContext(ctx, ws.String(), h)
	if err == nil {
		return conn, nil
	}
	if resp == nil {
		return nil, transportError("terminalWS", err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		return nil, transportError("terminalWS", readErr)
	}
	e := apiError("terminalWS", resp, body)
	if resp.StatusCode == http.StatusForbidden && e.ServerCode != "err.protected_target" && e.ServerCode != "err.debug_shell.session_invalid" && !strings.HasPrefix(e.ServerCode, "err.host_self.") {
		if o.debugSession != "" {
			e.Message += "; a debug shell needs containers.debug_shell"
		} else {
			e.Message += "; a shell needs terminal.access or containers.manage"
		}
	}
	return nil, e
}

type terminalFrame struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// shellSession pumps in to the shell and the shell to out until either side
// closes. The server sends output as binary frames and its own notices, the
// banner and errors, as text.
func shellSession(ctx context.Context, conn *websocket.Conn, in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var writeMu sync.Mutex
	send := func(f terminalFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(f)
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := in.Read(buf)
			if n > 0 && send(terminalFrame{Type: "input", Data: string(buf[:n])}) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		last := [2]int{}
		t := time.NewTicker(resizePoll)
		defer t.Stop()
		for {
			if cols, rows, ok := terminalSize(); ok && [2]int{cols, rows} != last {
				last = [2]int{cols, rows}
				if send(terminalFrame{Type: "resize", Cols: cols, Rows: rows}) != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	for {
		kind, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil || websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
				return nil
			}
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return nil
			}
			return transportError("terminalWS", err)
		}
		if kind == websocket.TextMessage {
			var f terminalFrame
			if json.Unmarshal(msg, &f) == nil && f.Type == "error" {
				return &Error{Code: ExitRemote, Operation: "terminalWS", Message: f.Data}
			}
		}
		if _, err := out.Write(msg); err != nil {
			return err
		}
	}
}

// dialConsole opens the guest console WebSocket.
//
// The route is public by design and the single-use 30-second ticket is the
// credential, because a browser cannot set a header on an upgrade — so this
// deliberately sends no Authorization header and relies on the ticket the caller
// has just redeemed.
func (a *API) dialConsole(ctx context.Context, ticket string) (*websocket.Conn, error) {
	base, err := url.Parse(strings.TrimRight(a.target.URL, "/"))
	if err != nil {
		return nil, usageError("invalid server URL %q: %v", a.target.URL, err)
	}
	origin := base.Scheme + "://" + base.Host
	ws := *base
	switch base.Scheme {
	case "https":
		ws.Scheme = "wss"
	case "http":
		ws.Scheme = "ws"
	default:
		return nil, usageError("the server URL must be http or https, not %q", base.Scheme)
	}
	ws.Path = base.Path + "/api/compute/console/" + url.PathEscape(ticket)

	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 30 * time.Second}
	if t, ok := a.http.Transport.(*http.Transport); ok {
		dialer.TLSClientConfig = t.TLSClientConfig
	}
	conn, resp, err := dialer.DialContext(ctx, ws.String(),
		http.Header{"User-Agent": {"mcharbor-cli/" + Version}, "Origin": {origin}})
	if err == nil {
		return conn, nil
	}
	if resp == nil {
		return nil, transportError("computeConsoleWS", err)
	}
	defer func() { warnErr(resp.Body.Close()) }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		return nil, transportError("computeConsoleWS", readErr)
	}
	return nil, apiError("computeConsoleWS", resp, body)
}
