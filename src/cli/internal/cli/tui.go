// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// tuiPoll is how often the screen asks the server for the container list. Live
// state is the reason the screen exists, and two seconds is close enough to live
// without turning a browsing session into a load test.
var tuiPoll = 2 * time.Second

// addTUICommand registers the interactive screen. It is one screen on purpose:
// the container list for the current context, with the same four lifecycle
// actions and the same confirmation rules the non-interactive commands enforce.
func addTUICommand(root *cobra.Command, g *globals) {
	root.AddCommand(&cobra.Command{
		Use:     "tui",
		Aliases: []string{"ui"},
		Short:   "Browse and operate the current environment's containers on one screen",
		Long: "tui opens a full-screen container list for the environment the context selects, refreshed while " +
			"it is open. j/k move, / filters, s starts, t stops, r restarts and d removes.\n\n" +
			"The confirmation rules are the ones the commands document, not softer ones: a stop or a restart " +
			"asks y/N, and a remove makes you type the container's name back, because being in front of the " +
			"screen is not a reason to make an irreversible change one keystroke away. --yes and --confirm " +
			"behave here exactly as they do elsewhere, which means --yes still does not cover a remove.\n\n" +
			"It needs a terminal, and containers.view plus the permission each action needs.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !stdinIsTerminal() {
				return usageError("the interactive screen needs a terminal; use mcharbor container list for a pipe")
			}
			a, err := g.api()
			if err != nil {
				return err
			}
			start := tuiProgram{
				runner: &tuiRunner{g: g, api: a},
				model:  tuiModel{loading: true, yes: g.yes, confirmName: g.confirmName, env: a.target.Env},
			}
			program := tea.NewProgram(start, tea.WithAltScreen(), tea.WithContext(cmd.Context()))
			final, err := program.Run()
			if err != nil {
				return &Error{Code: ExitFailure, Message: "the interactive screen ended badly: " + err.Error()}
			}
			if done, ok := final.(tuiProgram); ok && done.model.failure != "" {
				emitln(g.stderr, done.model.failure)
			}
			return nil
		},
	})
}

// tuiRunner is the impure half: it owns the client and turns a tuiAction into a
// request. Nothing in tui_model.go can reach it, which is the point.
type tuiRunner struct {
	g   *globals
	api *API
}

// list fetches the container list, mapping the Docker shape onto the rows the
// screen shows.
func (r *tuiRunner) list(ctx context.Context) tea.Msg {
	resp, err := r.api.c.ContainersList(ctx, &client.ContainersListParams{Env: r.api.env()})
	data, err := r.api.do("containersList", resp, err)
	if err != nil {
		return tuiErrMsg(asError(err).Error())
	}
	v, err := decodeAny(data)
	if err != nil {
		return tuiErrMsg(asError(err).Error())
	}
	items := listItems(v)
	rows := make([]tuiRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, tuiRow{
			ID:     shortID(item["Id"]),
			Name:   firstName(item["Names"]),
			Image:  render(item["Image"]),
			State:  render(item["State"]),
			Status: render(item["Status"]),
		})
	}
	return tuiRowsMsg(rows)
}

// mutate performs one lifecycle call. The server writes the audit row, exactly as
// it does for the same action typed as a command.
func (r *tuiRunner) mutate(ctx context.Context, action, target string) tea.Msg {
	op := "containers" + capitalise(action)
	var doErr error
	switch action {
	case "start":
		resp, err := r.api.c.ContainersStart(ctx, target, &client.ContainersStartParams{Env: r.api.env()})
		_, doErr = r.api.do(op, resp, err)
	case "stop":
		resp, err := r.api.c.ContainersStop(ctx, target, &client.ContainersStopParams{Env: r.api.env()})
		_, doErr = r.api.do(op, resp, err)
	case "restart":
		resp, err := r.api.c.ContainersRestart(ctx, target, &client.ContainersRestartParams{Env: r.api.env()})
		_, doErr = r.api.do(op, resp, err)
	case "remove":
		resp, err := r.api.c.ContainersRemove(ctx, target, &client.ContainersRemoveParams{Env: r.api.env()})
		_, doErr = r.api.do(op, resp, err)
	default:
		return tuiDoneMsg{Action: action, Target: target, Err: "unknown action " + action}
	}
	if doErr != nil {
		return tuiDoneMsg{Action: action, Target: target, Err: asError(doErr).Error()}
	}
	return tuiDoneMsg{Action: action, Target: target}
}

// tuiProgram adapts the pure model to bubbletea: it holds the runner and turns
// each tuiAction into a tea.Cmd.
type tuiProgram struct {
	runner *tuiRunner
	model  tuiModel
}

// Init loads the first list and starts the poll.
func (p tuiProgram) Init() tea.Cmd {
	return tea.Batch(p.fetch(), tuiTick())
}

// Update runs the pure update and performs whatever it asked for.
func (p tuiProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, action := tuiUpdate(p.model, msg)
	p.model = next
	var cmds []tea.Cmd
	if _, isTick := msg.(tuiTickMsg); isTick {
		cmds = append(cmds, tuiTick())
	}
	switch action.Kind {
	case tuiQuit:
		return p, tea.Quit
	case tuiRefresh:
		cmds = append(cmds, p.fetch())
	case tuiMutate:
		cmds = append(cmds, p.call(action.Action, action.Target))
	}
	return p, tea.Batch(cmds...)
}

func (p tuiProgram) fetch() tea.Cmd {
	runner := p.runner
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return runner.list(ctx)
	}
}

func (p tuiProgram) call(action, target string) tea.Cmd {
	runner := p.runner
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return runner.mutate(ctx, action, target)
	}
}

func tuiTick() tea.Cmd {
	return tea.Tick(tuiPoll, func(time.Time) tea.Msg { return tuiTickMsg{} })
}

// View renders the screen. It is plain text on purpose: a container list is a
// table, and a table that reads the same in a screenshot and in a pipe is worth
// more than a styled one.
func (p tuiProgram) View() string {
	m := p.model
	var b strings.Builder
	scope := m.env
	if scope == "" {
		scope = "the context's default environment"
	}
	fmt.Fprintf(&b, "mcharbor — containers in %s\n\n", scope)

	rows := m.visible()
	if len(rows) == 0 {
		switch {
		case m.loading:
			b.WriteString("  loading…\n")
		case m.filter != "":
			fmt.Fprintf(&b, "  no container matches %q\n", m.filter)
		default:
			b.WriteString("  no containers\n")
		}
	}
	for i, row := range rows {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%-14s %-28s %-10s %-24s %s\n",
			marker, row.ID, truncateCell(row.Name, 28), row.State, truncateCell(row.Image, 24), row.Status)
	}

	b.WriteString("\n")
	switch m.mode {
	case tuiFiltering:
		fmt.Fprintf(&b, "filter: %s_   (enter keeps it, esc clears it)\n", m.filter)
	case tuiConfirming:
		if m.pending.Need == needTypedName {
			fmt.Fprintf(&b, "%s %s cannot be undone. Type %q to confirm: %s_   (esc aborts)\n",
				capitalise(m.pending.Action), m.pending.Target, m.pending.Target, m.pending.Typed)
		} else {
			fmt.Fprintf(&b, "%s %s? [y/N]   (esc aborts)\n", capitalise(m.pending.Action), m.pending.Target)
		}
	default:
		if m.filter != "" {
			fmt.Fprintf(&b, "filter: %s   (esc clears it)\n", m.filter)
		}
		b.WriteString("j/k move · / filter · s start · t stop · r restart · d remove · R refresh · q quit\n")
	}
	if m.status != "" {
		fmt.Fprintf(&b, "%s\n", m.status)
	}
	if m.failure != "" {
		fmt.Fprintf(&b, "%s\n", m.failure)
	}
	return b.String()
}

// truncateCell keeps a column narrow without hiding which row it belongs to; the
// full value is always available from container get.
func truncateCell(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}
