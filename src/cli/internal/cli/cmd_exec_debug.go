// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// debugInstall is the server's answer to POST /containers/{id}/debug-shell.
type debugInstall struct {
	SessionID     string `json:"sessionId"`
	Path          string `json:"path"`
	Arch          string `json:"arch"`
	BusyboxDigest string `json:"busyboxDigest"`
	SizeBytes     int64  `json:"sizeBytes"`
	Installed     bool   `json:"installed"`
	ExpiresAt     string `json:"expiresAt"`
}

func debugShellPath(target string) string {
	return "/api/containers/" + url.PathEscape(target) + "/debug-shell"
}

func (a *API) envQuery() url.Values {
	q := url.Values{}
	if a.target.Env != "" {
		q.Set("env", a.target.Env)
	}
	return q
}

// withDebugPermission names the key a refused install needs; the generated
// permission table does not know a route that openapi.json has not caught up with.
func withDebugPermission(err error) error {
	var e *Error
	if errors.As(err, &e) && e.Code == ExitForbidden && e.ServerCode != "err.protected_target" && len(e.Permissions) == 0 {
		hint := *e
		hint.Permissions = []string{"containers.debug_shell"}
		return &hint
	}
	return err
}

func (g *globals) installDebugShell(cmd *cobra.Command, a *API, target string) (*debugInstall, error) {
	ctx, cancel := g.ctx(cmd)
	defer cancel()
	resp, err := a.rawRequest(ctx, http.MethodPost, debugShellPath(target), a.envQuery(), nil)
	data, err := a.do("containersDebugShellInstall", resp, err)
	if err != nil {
		return nil, withDebugPermission(err)
	}
	var d debugInstall
	if err := json.Unmarshal(data, &d); err != nil || d.SessionID == "" || d.Path == "" {
		return nil, &Error{Code: ExitRemote, Operation: "containersDebugShellInstall", Message: "the server did not return a debug session"}
	}
	return &d, nil
}

// removeDebugShell ends the session now rather than leaving it to expiry. It runs
// on its own context, because the command's may already be cancelled by Ctrl-C.
func (g *globals) removeDebugShell(a *API, target, session string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q := a.envQuery()
	q.Set("session", session)
	resp, err := a.rawRequest(ctx, http.MethodDelete, debugShellPath(target), q, nil)
	_, err = a.do("containersDebugShellRemove", resp, err)
	return err
}

func (g *globals) announceDebugShell(target string, d *debugInstall) {
	verb := "installed"
	if !d.Installed {
		verb = "reused"
	}
	emitf(g.stderr, "%s: busybox %s at %s (%s, %s, %d bytes); removed when this session ends, at the latest %s\n",
		target, verb, d.Path, d.Arch, d.BusyboxDigest, d.SizeBytes, d.ExpiresAt)
}

func (g *globals) runDebugShell(cmd *cobra.Command, target string, o execOptions) error {
	if !stdinIsTerminal() {
		return usageError("an interactive shell needs a terminal; to run one command, put it after --")
	}
	a, err := g.api()
	if err != nil {
		return err
	}
	d, err := g.installDebugShell(cmd, a, target)
	if err != nil {
		return err
	}
	g.announceDebugShell(target, d)
	o.debugSession = d.SessionID
	conn, err := a.dialTerminal(cmd.Context(), target, o)
	if err != nil {
		if rmErr := g.removeDebugShell(a, target, d.SessionID); rmErr != nil {
			emitf(g.stderr, "%s: removing the debug shell failed; it expires at %s: %v\n", target, d.ExpiresAt, rmErr)
		}
		return err
	}
	defer func() { warnErr(conn.Close()) }()
	restore, err := makeRaw()
	if err != nil {
		return usageError("switching the terminal to raw mode: %v", err)
	}
	defer restore()
	return shellSession(cmd.Context(), conn, stdin, g.stdout)
}

func (g *globals) runDebugExec(cmd *cobra.Command, target string, command []string, o execOptions) error {
	a, err := g.api()
	if err != nil {
		return err
	}
	d, err := g.installDebugShell(cmd, a, target)
	if err != nil {
		return err
	}
	g.announceDebugShell(target, d)
	runErr := g.runExec(cmd, target, append([]string{d.Path}, command...), execOptions{user: o.user, workDir: o.workDir})
	if rmErr := g.removeDebugShell(a, target, d.SessionID); rmErr != nil {
		emitf(g.stderr, "%s: removing the debug shell failed; it expires at %s: %v\n", target, d.ExpiresAt, rmErr)
		if runErr == nil {
			return rmErr
		}
	}
	return runErr
}
