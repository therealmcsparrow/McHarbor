// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// tuiKeyMsg builds the keystroke the screen sees. Named keys go through their
// own Key types because that is how the terminal delivers them.
func tuiKeyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg(tea.Key{Type: tea.KeyEnter})
	case "esc":
		return tea.KeyMsg(tea.Key{Type: tea.KeyEsc})
	case "backspace":
		return tea.KeyMsg(tea.Key{Type: tea.KeyBackspace})
	case "up":
		return tea.KeyMsg(tea.Key{Type: tea.KeyUp})
	case "down":
		return tea.KeyMsg(tea.Key{Type: tea.KeyDown})
	case "ctrl+c":
		return tea.KeyMsg(tea.Key{Type: tea.KeyCtrlC})
	}
	return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune(s)})
}

// tuiType pushes each key through the update in order and returns the last
// action produced, so a test can assert that typing produced nothing until the
// final keystroke.
func tuiType(m tuiModel, keys ...string) (tuiModel, []tuiAction) {
	actions := make([]tuiAction, 0, len(keys))
	for _, k := range keys {
		next, action := tuiUpdate(m, tuiKeyMsg(k))
		m = next
		actions = append(actions, action)
	}
	return m, actions
}

func tuiFixture() tuiModel {
	return tuiModel{
		rows: []tuiRow{
			{ID: "aaa111", Name: "web", Image: "nginx:1", State: "running", Status: "Up 2 hours"},
			{ID: "bbb222", Name: "db", Image: "postgres:17", State: "running", Status: "Up 3 days"},
			{ID: "ccc333", Name: "worker", Image: "app:1", State: "exited", Status: "Exited (0)"},
		},
	}
}

func onlyMutations(actions []tuiAction) []tuiAction {
	var out []tuiAction
	for _, a := range actions {
		if a.Kind == tuiMutate {
			out = append(out, a)
		}
	}
	return out
}

// TestTUIUpdateIsPure: the same model and message always produce the same result,
// and the input model is not modified. Every guarantee below rests on this.
func TestTUIUpdateIsPure(t *testing.T) {
	start := tuiFixture()
	start.cursor = 1

	first, actionA := tuiUpdate(start, tuiKeyMsg("d"))
	second, actionB := tuiUpdate(start, tuiKeyMsg("d"))

	if actionA != actionB {
		t.Fatalf("the same keystroke produced two different actions: %+v vs %+v", actionA, actionB)
	}
	if first.mode != second.mode || first.pending != second.pending || first.cursor != second.cursor {
		t.Fatalf("the same keystroke produced two different models: %+v vs %+v", first, second)
	}
	if start.mode != tuiBrowse || start.pending != (tuiPending{}) || start.cursor != 1 {
		t.Fatalf("update mutated the model it was given: %+v", start)
	}
}

// TestTUIRemoveRequiresTheTypedName is the guarantee the screen exists to keep.
// remove is irreversible, so being in front of the screen must not make it one
// keystroke, and no request may be issued until the name matches exactly.
func TestTUIRemoveRequiresTheTypedName(t *testing.T) {
	m := tuiFixture()

	m, actions := tuiType(m, "d")
	if got := onlyMutations(actions); len(got) != 0 {
		t.Fatalf("d alone issued a request: %+v", got)
	}
	if m.mode != tuiConfirming || m.pending.Need != needTypedName {
		t.Fatalf("d must open a typed-name confirmation, got mode %v need %v", m.mode, m.pending.Need)
	}
	if m.pending.Action != "remove" || m.pending.Target != "web" {
		t.Fatalf("the pending action must name what it will do: %+v", m.pending)
	}

	// The name typed wrong: aborted, and still nothing issued.
	wrong, actions := tuiType(m, "w", "e", "p", "enter")
	if got := onlyMutations(actions); len(got) != 0 {
		t.Fatalf("a mismatched name issued a request: %+v", got)
	}
	if wrong.mode != tuiBrowse {
		t.Fatalf("a mismatched name must return to the list, got mode %v", wrong.mode)
	}
	if !strings.Contains(wrong.failure, "did not match") {
		t.Fatalf("a mismatched name must say so, got %q", wrong.failure)
	}

	// Escape at any point aborts and issues nothing.
	escaped, actions := tuiType(m, "w", "e", "esc")
	if got := onlyMutations(actions); len(got) != 0 {
		t.Fatalf("esc issued a request: %+v", got)
	}
	if escaped.mode != tuiBrowse || escaped.failure != "aborted" {
		t.Fatalf("esc must abort, got mode %v failure %q", escaped.mode, escaped.failure)
	}

	// The name typed exactly: one remove, for that container.
	confirmed, actions := tuiType(m, "w", "e", "b", "enter")
	got := onlyMutations(actions)
	if len(got) != 1 {
		t.Fatalf("the typed name must issue exactly one request, got %+v", got)
	}
	if got[0].Action != "remove" || got[0].Target != "web" {
		t.Fatalf("the request must be remove web, got %+v", got[0])
	}
	if confirmed.mode != tuiBrowse || confirmed.pending != (tuiPending{}) {
		t.Fatalf("a confirmed action must clear the pending state: %+v", confirmed)
	}
}

// TestTUIYesDoesNotCoverARemove: --yes covers a disruptive change and never an
// irreversible one, on the screen exactly as on the command line.
func TestTUIYesDoesNotCoverARemove(t *testing.T) {
	m := tuiFixture()
	m.yes = true

	after, actions := tuiType(m, "d")
	if got := onlyMutations(actions); len(got) != 0 {
		t.Fatalf("--yes shortcut a remove: %+v", got)
	}
	if after.pending.Need != needTypedName {
		t.Fatalf("--yes must still require the typed name, got need %v", after.pending.Need)
	}

	// --confirm naming the target is the one thing that does cover it, because
	// that is what confirmNeeded says for a command.
	named := tuiFixture()
	named.confirmName = "web"
	_, actions = tuiType(named, "d")
	if got := onlyMutations(actions); len(got) != 1 || got[0].Action != "remove" {
		t.Fatalf("--confirm web must cover remove web, got %+v", got)
	}
}

// TestTUIDisruptiveActionsAskYesNo: stop and restart interrupt something, so they
// ask, and anything but y aborts. start is reversible and does not ask at all.
func TestTUIDisruptiveActionsAskYesNo(t *testing.T) {
	for _, tc := range []struct{ key, action string }{{"t", "stop"}, {"r", "restart"}} {
		m := tuiFixture()
		m, actions := tuiType(m, tc.key)
		if got := onlyMutations(actions); len(got) != 0 {
			t.Fatalf("%s issued a request before the y/N answer: %+v", tc.key, got)
		}
		if m.mode != tuiConfirming || m.pending.Need != needYesNo || m.pending.Action != tc.action {
			t.Fatalf("%s must open a y/N confirmation for %s, got %+v", tc.key, tc.action, m.pending)
		}
		if _, actions := tuiType(m, "n"); len(onlyMutations(actions)) != 0 {
			t.Fatalf("n must abort %s", tc.action)
		}
		got := onlyMutations(mustActions(tuiType(m, "y")))
		if len(got) != 1 || got[0].Action != tc.action || got[0].Target != "web" {
			t.Fatalf("y must issue %s web, got %+v", tc.action, got)
		}
	}

	m := tuiFixture()
	after, actions := tuiType(m, "s")
	got := onlyMutations(actions)
	if len(got) != 1 || got[0].Action != "start" {
		t.Fatalf("start is reversible and must run without asking, got %+v", got)
	}
	if after.mode != tuiBrowse {
		t.Fatalf("start must not open a confirmation, got mode %v", after.mode)
	}
}

// TestTUIFilterNarrowsAndClampsTheCursor: a filter that leaves the cursor past
// the end of the list would act on whatever row happened to be there.
func TestTUIFilterNarrowsAndClampsTheCursor(t *testing.T) {
	m := tuiFixture()
	m.cursor = 2

	m, _ = tuiType(m, "/", "w", "e", "b")
	if m.mode != tuiFiltering || m.filter != "web" {
		t.Fatalf("/ must start a filter, got mode %v filter %q", m.mode, m.filter)
	}
	rows := m.visible()
	if len(rows) != 1 || rows[0].Name != "web" {
		t.Fatalf("the filter must narrow the list, got %+v", rows)
	}
	if m.cursor != 0 {
		t.Fatalf("the cursor must be clamped into the filtered list, got %d", m.cursor)
	}
	row, ok := m.selected()
	if !ok || row.Name != "web" {
		t.Fatalf("the selection must follow the filter, got %+v ok=%v", row, ok)
	}

	// esc while filtering clears it; the full list comes back.
	cleared, _ := tuiType(m, "esc")
	if cleared.filter != "" || len(cleared.visible()) != 3 {
		t.Fatalf("esc must clear the filter, got %q with %d rows", cleared.filter, len(cleared.visible()))
	}

	// A filter matching nothing selects nothing, and an action on nothing is a
	// no-op rather than an action on row zero.
	empty := tuiFixture()
	empty.filter = "nothing-matches-this"
	if _, ok := empty.selected(); ok {
		t.Fatal("an empty filtered list must select nothing")
	}
	if _, actions := tuiType(empty, "d"); len(onlyMutations(actions)) != 0 {
		t.Fatal("an action with nothing selected must issue nothing")
	}
}

// TestTUIPollDoesNotInterruptAConfirmation: the list is what the cursor points
// into, so refreshing it while a name is being typed would change the target.
func TestTUIPollDoesNotInterruptAConfirmation(t *testing.T) {
	m := tuiFixture()
	m, _ = tuiType(m, "d")

	after, action := tuiUpdate(m, tuiTickMsg{})
	if action.Kind != tuiNothing {
		t.Fatalf("a poll during a confirmation must do nothing, got %+v", action)
	}
	if after.mode != tuiConfirming || after.pending != m.pending {
		t.Fatalf("a poll must leave the confirmation alone: %+v", after.pending)
	}

	// Outside a confirmation the poll does refresh.
	browse := tuiFixture()
	if _, action := tuiUpdate(browse, tuiTickMsg{}); action.Kind != tuiRefresh {
		t.Fatalf("a poll in the list must refresh, got %+v", action)
	}
}

// TestTUIServerRefusalArrivesIntact: a 403 or a conflict is the operator's only
// explanation, so the screen shows the server's own message rather than a
// generic failure.
func TestTUIServerRefusalArrivesIntact(t *testing.T) {
	m := tuiFixture()
	refusal := "containersRemove: You do not have permission to perform this action (HTTP 403) " +
		"[err.rbac.permission_denied]; missing permission: containers.delete"

	after, action := tuiUpdate(m, tuiDoneMsg{Action: "remove", Target: "web", Err: refusal})
	if after.failure != refusal {
		t.Fatalf("the server's message must arrive intact, got %q", after.failure)
	}
	if action.Kind != tuiRefresh {
		t.Fatalf("a finished action must refresh the list, got %+v", action)
	}

	ok, _ := tuiUpdate(m, tuiDoneMsg{Action: "stop", Target: "web"})
	if ok.failure != "" || !strings.Contains(ok.status, "stopped") {
		t.Fatalf("a successful action must report it, got status %q failure %q", ok.status, ok.failure)
	}
}

// mustActions is the second return of tuiType, for the cases where the model
// afterwards does not matter.
func mustActions(_ tuiModel, actions []tuiAction) []tuiAction { return actions }
