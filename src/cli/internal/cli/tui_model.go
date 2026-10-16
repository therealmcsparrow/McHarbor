// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// tuiMode is which of the screen's three input states is current. The mode
// decides what a keystroke means, which is why it is a field rather than a
// scattering of booleans.
type tuiMode int

const (
	// tuiBrowse is the list: movement keys and action keys.
	tuiBrowse tuiMode = iota
	// tuiFiltering is the filter line: every rune is filter text.
	tuiFiltering
	// tuiConfirming is a pending action waiting for the confirmation its risk
	// level demands.
	tuiConfirming
)

// tuiRow is one container as the screen shows it.
type tuiRow struct {
	ID     string
	Name   string
	Image  string
	State  string
	Status string
}

// tuiActionKind is what the pure update asks the program to do next.
type tuiActionKind int

const (
	// tuiNothing is the common case: the keystroke only changed the view.
	tuiNothing tuiActionKind = iota
	// tuiRefresh asks for a fresh container list.
	tuiRefresh
	// tuiMutate asks for one lifecycle call. It is produced only after the
	// confirmation the action's risk level demands has been given.
	tuiMutate
	// tuiQuit ends the program.
	tuiQuit
)

// tuiAction is the update function's only output besides the new model. Keeping
// the effect a value rather than a call is what makes the update pure and lets a
// test assert that a refused confirmation issues nothing at all.
type tuiAction struct {
	Kind   tuiActionKind
	Action string
	Target string
}

// tuiLifecycle maps a keystroke to a lifecycle action and its risk. The risks
// are the same constants container start/stop/restart/remove use, so the screen
// cannot be gentler than the command.
var tuiLifecycle = map[string]struct {
	action string
	risk   Risk
}{
	"s": {"start", Reversible},
	"t": {"stop", Disruptive},
	"r": {"restart", Disruptive},
	"d": {"remove", Irreversible},
}

// tuiPending is one action held for confirmation.
type tuiPending struct {
	Action string
	Target string
	Need   confirmNeed
	Typed  string
}

// tuiModel is the whole screen's state. Every field is a value, so a test can
// build one, push a message through and compare.
type tuiModel struct {
	rows    []tuiRow
	filter  string
	cursor  int
	mode    tuiMode
	pending tuiPending
	status  string
	failure string
	loading bool
	width   int
	height  int

	// yes and confirmName mirror the global --yes and --confirm flags. They are
	// carried into the screen so confirmNeeded gives the same verdict here as it
	// gives a non-interactive command.
	yes         bool
	confirmName string
	env         string
}

// tuiRowsMsg delivers a fresh container list.
type tuiRowsMsg []tuiRow

// tuiErrMsg delivers a message the screen should show instead of a list.
type tuiErrMsg string

// tuiTickMsg is the poll: the screen asks for a refresh rather than timing one.
type tuiTickMsg struct{}

// tuiDoneMsg reports one completed lifecycle call. Err is the server's own
// message when it refused, which is what the operator has to read.
type tuiDoneMsg struct {
	Action string
	Target string
	Err    string
}

// visible returns the rows the filter admits, matched case-insensitively against
// the name, image, state and id.
func (m tuiModel) visible() []tuiRow {
	if m.filter == "" {
		return m.rows
	}
	needle := strings.ToLower(m.filter)
	out := make([]tuiRow, 0, len(m.rows))
	for _, row := range m.rows {
		haystack := strings.ToLower(row.Name + " " + row.Image + " " + row.State + " " + row.ID)
		if strings.Contains(haystack, needle) {
			out = append(out, row)
		}
	}
	return out
}

// selected returns the row under the cursor, and false when the list is empty.
func (m tuiModel) selected() (tuiRow, bool) {
	rows := m.visible()
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return tuiRow{}, false
	}
	return rows[m.cursor], true
}

// clampCursor keeps the cursor inside the filtered list after any change to it.
func (m tuiModel) clampCursor() tuiModel {
	n := len(m.visible())
	switch {
	case n == 0:
		m.cursor = 0
	case m.cursor >= n:
		m.cursor = n - 1
	case m.cursor < 0:
		m.cursor = 0
	}
	return m
}

// tuiUpdate is the screen's whole behaviour, as a pure function: it takes a
// model and a message and returns the next model and the effect the program
// should perform. Nothing here reads a clock, a socket or the filesystem, which
// is what makes the confirmation guarantees testable without timing.
func tuiUpdate(m tuiModel, msg tea.Msg) (tuiModel, tuiAction) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		return m, tuiAction{}

	case tuiRowsMsg:
		m.rows = []tuiRow(typed)
		m.loading = false
		m.failure = ""
		return m.clampCursor(), tuiAction{}

	case tuiErrMsg:
		m.loading = false
		m.failure = string(typed)
		return m, tuiAction{}

	case tuiTickMsg:
		// A poll must never interrupt a confirmation: the list under the cursor
		// would change while the operator is typing a name into it.
		if m.mode == tuiConfirming {
			return m, tuiAction{}
		}
		m.loading = true
		return m, tuiAction{Kind: tuiRefresh}

	case tuiDoneMsg:
		if typed.Err != "" {
			m.failure = typed.Err
		} else {
			m.status = typed.Target + ": " + pastTense(typed.Action)
			m.failure = ""
		}
		m.loading = true
		return m, tuiAction{Kind: tuiRefresh}

	case tea.KeyMsg:
		return tuiKey(m, typed.String())
	}
	return m, tuiAction{}
}

// tuiKey is the keystroke half of the update, split out because the mode
// dispatch is the part worth reading on its own.
func tuiKey(m tuiModel, key string) (tuiModel, tuiAction) {
	switch m.mode {
	case tuiFiltering:
		switch key {
		case "enter", "esc":
			m.mode = tuiBrowse
			if key == "esc" {
				m.filter = ""
			}
			return m.clampCursor(), tuiAction{}
		case "backspace":
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
			}
			return m.clampCursor(), tuiAction{}
		case "ctrl+c":
			return m, tuiAction{Kind: tuiQuit}
		}
		if len([]rune(key)) == 1 {
			m.filter += key
		}
		return m.clampCursor(), tuiAction{}

	case tuiConfirming:
		return tuiConfirmKey(m, key)
	}

	switch key {
	case "q", "ctrl+c":
		return m, tuiAction{Kind: tuiQuit}
	case "/":
		m.mode = tuiFiltering
		m.status, m.failure = "", ""
		return m, tuiAction{}
	case "esc":
		m.filter, m.status, m.failure = "", "", ""
		return m.clampCursor(), tuiAction{}
	case "up", "k":
		m.cursor--
		return m.clampCursor(), tuiAction{}
	case "down", "j":
		m.cursor++
		return m.clampCursor(), tuiAction{}
	case "g", "home":
		m.cursor = 0
		return m, tuiAction{}
	case "G", "end":
		m.cursor = len(m.visible()) - 1
		return m.clampCursor(), tuiAction{}
	case "R", "ctrl+r":
		m.loading = true
		return m, tuiAction{Kind: tuiRefresh}
	}

	lifecycle, ok := tuiLifecycle[key]
	if !ok {
		return m, tuiAction{}
	}
	row, ok := m.selected()
	if !ok {
		return m, tuiAction{}
	}
	target := row.Name
	if target == "" {
		target = row.ID
	}
	need := confirmNeeded(lifecycle.risk, target, m.yes, m.confirmName)
	if need == needNothing {
		m.status, m.failure = "", ""
		return m, tuiAction{Kind: tuiMutate, Action: lifecycle.action, Target: target}
	}
	m.mode = tuiConfirming
	m.pending = tuiPending{Action: lifecycle.action, Target: target, Need: need}
	m.status, m.failure = "", ""
	return m, tuiAction{}
}

// tuiConfirmKey answers the pending confirmation. The wording of a refusal is
// the CLI's own, so the same mistake reads the same way in both.
func tuiConfirmKey(m tuiModel, key string) (tuiModel, tuiAction) {
	pending := m.pending
	abort := func(reason string) (tuiModel, tuiAction) {
		m.mode = tuiBrowse
		m.pending = tuiPending{}
		m.failure = reason
		return m, tuiAction{}
	}

	if key == "esc" || key == "ctrl+c" {
		return abort("aborted")
	}

	if pending.Need == needYesNo {
		if key == "y" || key == "Y" {
			m.mode = tuiBrowse
			m.pending = tuiPending{}
			return m, tuiAction{Kind: tuiMutate, Action: pending.Action, Target: pending.Target}
		}
		return abort("aborted")
	}

	switch key {
	case "enter":
		if pending.Typed != pending.Target {
			return abort("aborted: the name did not match")
		}
		m.mode = tuiBrowse
		m.pending = tuiPending{}
		return m, tuiAction{Kind: tuiMutate, Action: pending.Action, Target: pending.Target}
	case "backspace":
		if pending.Typed != "" {
			m.pending.Typed = pending.Typed[:len(pending.Typed)-1]
		}
		return m, tuiAction{}
	}
	if len([]rune(key)) == 1 {
		m.pending.Typed += key
	}
	return m, tuiAction{}
}
