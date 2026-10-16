// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// msgCaseIdents collects every Msg* identifier used as a case value in the
// switches of the named files whose body satisfies keep.
func msgCaseIdents(t *testing.T, files []string, keep func(*ast.CaseClause) bool) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok || !keep(cc) {
				return true
			}
			for _, e := range cc.List {
				if id, ok := e.(*ast.Ident); ok && strings.HasPrefix(id.Name, "Msg") {
					out[id.Name] = true
				}
			}
			return true
		})
	}
	return out
}

// callsHandleComputeMessage reports whether a case body forwards to the compute runtime.
func callsHandleComputeMessage(cc *ast.CaseClause) bool {
	found := false
	for _, st := range cc.Body {
		ast.Inspect(st, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "handleComputeMessage" {
				found = true
			}
			return !found
		})
	}
	return found
}

// TestEveryHandledComputeMessageIsRouted guards the read loop's explicit case list: a type the
// compute dispatcher handles but the loop does not forward falls to the default branch and is
// refused as "no handler", which is how vm_spice_ticket and the hot-plug types shipped broken.
func TestEveryHandledComputeMessageIsRouted(t *testing.T) {
	handlerFiles, err := filepath.Glob("compute_*_linux.go")
	if err != nil || len(handlerFiles) == 0 {
		t.Fatalf("no compute handler files found: %v", err)
	}
	handled := msgCaseIdents(t, handlerFiles, func(*ast.CaseClause) bool { return true })
	routed := msgCaseIdents(t, []string{"agent.go"}, callsHandleComputeMessage)
	if len(routed) == 0 {
		t.Fatal("found no case in agent.go that calls handleComputeMessage")
	}

	// Console stream frames have their own cases in the read loop; reply types are
	// Agent->Server and never arrive here.
	ownCase := map[string]bool{"MsgConsoleStart": true, "MsgConsoleInput": true, "MsgConsoleResize": true, "MsgConsoleEnd": true}

	var missing []string
	for m := range handled {
		if routed[m] || ownCase[m] {
			continue
		}
		if strings.HasSuffix(m, "Reply") || strings.HasSuffix(m, "Confirmed") || strings.HasSuffix(m, "Response") ||
			strings.HasSuffix(m, "Progress") || strings.HasSuffix(m, "Event") || strings.HasSuffix(m, "Result") {
			continue
		}
		missing = append(missing, m)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("handled by compute_*_linux.go but not routed by agent.go's read loop: %v", missing)
	}
}
