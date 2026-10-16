// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build ruleguard

// Package gorules holds McHarbor's repository-specific lint rules, loaded by gocritic's ruleguard checker.
package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

// rawErrToClient flags an error's text reaching a response body, which leaks paths and untranslated English.
func rawErrToClient(m dsl.Matcher) {
	m.Match(`http.Error($_, $e.Error(), $_)`,
		`response.$f($*_, $e.Error(), $*_)`,
		`map[string]string{$*_, $_: $e.Error(), $*_}`,
		`$_{$*_, Detail: $e.Error(), $*_}`).
		Where(m["e"].Type.Implements("error") && !m.File().Name.Matches(`_test\.go$`)).
		Report("never return err.Error() to a client: log it, return an i18n code (api-routes.md rule 6)")
}

// sprintfIntoSQL flags a query string built with Sprintf in the query position of a non-context call.
func sprintfIntoSQL(m dsl.Matcher) {
	m.Match(`$db.$q(fmt.Sprintf($*_), $*_)`).
		Where(m["q"].Text.Matches(`^(Query|QueryRow|Exec|Prepare)$`) && !m.File().Name.Matches(`_test\.go$`)).
		Report("SQL built with Sprintf: use ? placeholders, or QuoteIdent plus a `// SQL identifier:` marker")
}

// sprintfIntoSQLContext is sprintfIntoSQL for the *Context forms, where the query is the second argument.
func sprintfIntoSQLContext(m dsl.Matcher) {
	m.Match(`$db.$q($_, fmt.Sprintf($*_), $*_)`).
		Where(m["q"].Text.Matches(`^(Query|QueryRow|Exec|Prepare)Context$`) && !m.File().Name.Matches(`_test\.go$`)).
		Report("SQL built with Sprintf: use ? placeholders, or QuoteIdent plus a `// SQL identifier:` marker")
}

// checkOriginTrue flags a websocket upgrader that accepts every origin.
//
// The match is on the func literal itself, not the enclosing `websocket.Upgrader{...}`
// composite literal: gocritic's ruleguard checker (v0.4.5, bundled with golangci-lint
// v2.14.0) does not match a KeyValueExpr field nested inside that literal, confirmed by
// break-to-prove. Both parameter forms are matched: `$_ *http.Request` binds a named
// parameter only, so the unnamed `*http.Request` form (the repo's actual instance, in
// core/nodelink/stream.go) needs its own alternative. Test files are excluded, mirroring
// rawErrToClient, because a mock WS server's permissive upgrader is not the anti-pattern.
func checkOriginTrue(m dsl.Matcher) {
	m.Match(`func($_ *http.Request) bool { return true }`,
		`func(*http.Request) bool { return true }`).
		Where(!m.File().Name.Matches(`_test\.go$`)).
		Report("CheckOrigin always true: validate Origin, or reject any Origin on a node-to-node endpoint")
}

// rawRowsAffected enforces the db.RowsAffected helper so the error is never dropped.
func rawRowsAffected(m dsl.Matcher) {
	m.Match(`$r.RowsAffected()`).Where(m["r"].Type.Is("sql.Result")).
		Report("use db.RowsAffected(result)")
}

// backgroundInRequestPath flags context.Background in handlers and services, where a caller ctx exists.
func backgroundInRequestPath(m dsl.Matcher) {
	m.Match(`context.Background()`).
		Where(m.File().Name.Matches(`^(handler.*|service)\.go$`)).
		Report("thread the caller's ctx; to outlive cancellation use context.WithoutCancel(ctx) plus WithTimeout")
}
