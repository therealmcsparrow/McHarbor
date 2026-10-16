// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

var containerColumns = []Column{
	{Header: "ID", Path: "Id", Format: shortID}, {Header: "NAME", Path: "Names", Format: firstName},
	{Header: "IMAGE", Path: "Image"}, {Header: "STATE", Path: "State"}, {Header: "STATUS", Path: "Status"},
	{Header: "STACK", Path: "StackName"},
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func addContainerCommands(root *cobra.Command, g *globals) {
	c := &cobra.Command{Use: "container", Aliases: []string{"containers", "ctr"}, Short: "Containers in the selected environment"}

	c.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List containers", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "containersList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainersList(ctx, &client.ContainersListParams{Env: a.env()})
			}, containerColumns)
		},
	})
	var reveal bool
	get := &cobra.Command{
		Use: "get <id|name>", Aliases: []string{"inspect"}, Short: "Show one container", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "containersInspect", func(ctx context.Context, a *API) (*http.Response, error) {
				params := &client.ContainersInspectParams{Env: a.env()}
				if reveal {
					params.Reveal = strPtr("true")
				}
				return a.c.ContainersInspect(ctx, args[0], params)
			}, []Column{
				{Header: "ID", Path: "Id", Format: shortID}, {Header: "Name", Path: "Name", Format: firstName},
				{Header: "Image", Path: "Config.Image"}, {Header: "State", Path: "State.Status"},
				{Header: "Started", Path: "State.StartedAt", Format: age}, {Header: "Restarts", Path: "RestartCount"},
				{Header: "Health", Path: "State.Health.Status"},
			})
		},
	}
	get.Flags().BoolVar(&reveal, "reveal", false, "show credential-looking env values in the clear (needs containers.manage; audit-logged)")
	c.AddCommand(get)

	lifecycle := []struct {
		use, short, op string
		risk           Risk
		call           func(ctx context.Context, a *API, id string) (*http.Response, error)
	}{
		{"start", "Start a container", "containersStart", Reversible, func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ContainersStart(ctx, id, &client.ContainersStartParams{Env: a.env()})
		}},
		{"restart", "Restart a container", "containersRestart", Disruptive, func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ContainersRestart(ctx, id, &client.ContainersRestartParams{Env: a.env()})
		}},
		{"pause", "Pause a container", "containersPause", Disruptive, func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ContainersPause(ctx, id, &client.ContainersPauseParams{Env: a.env()})
		}},
		{"unpause", "Unpause a container", "containersUnpause", Reversible, func(ctx context.Context, a *API, id string) (*http.Response, error) {
			return a.c.ContainersUnpause(ctx, id, &client.ContainersUnpauseParams{Env: a.env()})
		}},
	}
	for _, l := range lifecycle {
		l := l
		c.AddCommand(&cobra.Command{
			Use: l.use + " <id|name>", Short: l.short, Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return g.runAction(cmd, l.op, l.risk, l.use, args[0], func(ctx context.Context, a *API) (*http.Response, error) {
					return l.call(ctx, a, args[0])
				})
			},
		})
	}

	var stopWait int
	stop := &cobra.Command{
		Use: "stop <id|name>", Short: "Stop a container", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var t *string
			if cmd.Flags().Changed("time") {
				t = strPtr(strconv.Itoa(stopWait))
			}
			return g.runAction(cmd, "containersStop", Disruptive, "stop", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainersStop(ctx, args[0], &client.ContainersStopParams{Env: a.env(), T: t})
			})
		},
	}
	stop.Flags().IntVarP(&stopWait, "time", "t", 10, "seconds to wait before killing it")
	c.AddCommand(stop)

	var signal string
	kill := &cobra.Command{
		Use: "kill <id|name>", Short: "Send a signal to a container", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "containersKill", Disruptive, "kill", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainersKill(ctx, args[0], &client.ContainersKillParams{Env: a.env(), Signal: strPtr(signal)})
			})
		},
	}
	kill.Flags().StringVarP(&signal, "signal", "s", "", "signal to send; SIGKILL when empty")
	c.AddCommand(kill)

	var force, volumes bool
	remove := &cobra.Command{
		Use: "remove <id|name>", Aliases: []string{"rm"}, Short: "Remove a container (irreversible)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "containersRemove", Irreversible, "remove", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ContainersRemove(ctx, args[0], &client.ContainersRemoveParams{Env: a.env(), Force: boolParam(force), V: boolParam(volumes)})
			})
		},
	}
	remove.Flags().BoolVarP(&force, "force", "f", false, "kill it first if it is running")
	remove.Flags().BoolVar(&volumes, "volumes", false, "also remove its anonymous volumes")
	c.AddCommand(remove)

	c.AddCommand(logsCommand(g))
	c.AddCommand(execCommand(g))
	root.AddCommand(c)
}

// resumable are the reasons a log stream ends while the container keeps
// logging: the server's per-stream limits and its five-minute cutoff. eof means
// the container itself stopped, and following ends there.
var resumable = map[string]bool{"session_limit": true, "record_limit": true, "byte_limit": true, "cancelled": true, "": true}

func boolParam(b bool) *string {
	if !b {
		return nil
	}
	return strPtr("true")
}

// logsCommand prints a container's log, and with --follow keeps streaming. The
// server ends each stream after five minutes; the CLI resumes from the last
// event id, so a follow survives that with no lost or repeated lines.
func logsCommand(g *globals) *cobra.Command {
	var follow, timestamps bool
	var tail int
	cmd := &cobra.Command{
		Use: "logs <id|name>", Short: "Print, or with -f follow, a container's log", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := g.api()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if !follow {
				var cancel context.CancelFunc
				ctx, cancel = g.ctx(cmd)
				defer cancel()
			}
			cursor := ""
			for {
				q := url.Values{"tail": {strconv.Itoa(tail)}, "follow": {strconv.FormatBool(follow)}}
				if a.target.Env != "" {
					q.Set("env", a.target.Env)
				}
				headers := map[string]string{"Accept": "text/event-stream"}
				if cursor != "" {
					headers["Last-Event-ID"] = cursor
				}
				resp, err := a.rawGet(ctx, "/api/logs/"+url.PathEscape(args[0]), q, headers)
				if err != nil {
					return transportError("logsStreamLogs", err)
				}
				if resp.StatusCode >= 400 {
					body := make([]byte, 64*1024)
					n := valueOr(resp.Body.Read(body))
					warnErr(resp.Body.Close())
					return apiError("logsStreamLogs", resp, body[:n])
				}
				endReason := ""
				err = readSSE(resp.Body, func(ev sseEvent) bool {
					switch ev.Event {
					case "log":
						var rec struct {
							Timestamp string `json:"timestamp"`
							Message   string `json:"message"`
						}
						if json.Unmarshal([]byte(ev.Data), &rec) == nil {
							if timestamps {
								emitf(g.stdout, "%s %s\n", rec.Timestamp, rec.Message)
							} else {
								emitln(g.stdout, rec.Message)
							}
						}
						if ev.ID != "" {
							cursor = ev.ID
						}
					case "end":
						var end struct {
							Reason string `json:"reason"`
							Cursor string `json:"cursor"`
						}
						if json.Unmarshal([]byte(ev.Data), &end) == nil {
							endReason = end.Reason
							if end.Cursor != "" {
								cursor = end.Cursor
							}
						}
						return false
					}
					return true
				})
				warnErr(resp.Body.Close())
				if ctx.Err() != nil {
					return nil
				}
				if err != nil {
					return &Error{Code: ExitRemote, Message: "log stream interrupted: " + err.Error()}
				}
				if endReason == "failed" {
					return &Error{Code: ExitRemote, Message: "the server could not read the container's log; see the McHarbor log"}
				}
				if !follow || !resumable[endReason] {
					return nil
				}
				tail = 0
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(time.Second):
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep streaming new lines until interrupted")
	cmd.Flags().BoolVar(&timestamps, "timestamps", false, "prefix each line with its timestamp")
	cmd.Flags().IntVar(&tail, "tail", 100, "lines from the end to start with")
	return cmd
}
