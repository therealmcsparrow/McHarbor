// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// ABACExecutor evaluates admin-authored JavaScript expressions in a
// strict goja sandbox. Every call constructs a fresh runtime — no
// state, no closures across requests, no goroutine escape.
//
// Globals exposed (the *only* ones admins can call):
//   - hasLabel(map, key)             — safe map lookup
//   - inList(value, ...)             — variadic membership check
//   - now                            — frozen time object (precomputed)
//   - user, env, resource            — read-only attribute bundles
//
// Globals that are NOT exposed (refused at runtime):
//
//	require, fetch, process, XMLHttpRequest, setTimeout, setInterval,
//	Promise, then, queueMicrotask, console.
//
// goja rejects undeclared globals naturally; we never call vm.Set for
// them, so reading them throws a ReferenceError.
type ABACExecutor struct {
	timeout time.Duration
	logger  *slog.Logger
}

// NewABACExecutor returns an executor with a sane default timeout.
// Use WithTimeout to override per call.
func NewABACExecutor(logger *slog.Logger) *ABACExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &ABACExecutor{
		timeout: 50 * time.Millisecond,
		logger:  logger,
	}
}

// WithTimeout overrides the default 50ms timeout. Clamped to [1ms, 500ms].
func (e *ABACExecutor) WithTimeout(d time.Duration) *ABACExecutor {
	if d < time.Millisecond {
		d = time.Millisecond
	}
	if d > 500*time.Millisecond {
		d = 500 * time.Millisecond
	}
	c := *e
	c.timeout = d
	return &c
}

// Attributes is the read-only bundle exposed to policy expressions.
// All fields are shallow copies; resource.Labels is copied so admins
// cannot mutate the underlying map.
type Attributes struct {
	User     *UserAttributes     `json:"user"`
	Env      *EnvAttributes      `json:"env"`
	Resource *ResourceAttributes `json:"resource"`
	Now      *TimeAttributes     `json:"now"`
	Request  *RequestAttributes  `json:"request"`

	// envUnresolved is set when the environment lookup failed for a reason other
	// than a missing row: Env then carries only the id, and evalPolicies refuses to
	// let a policy that reads more than env.id grant anything.
	envUnresolved bool
}

// RequestAttributes describes the HTTP call being authorized, so a policy can tell
// an API key from a browser session or a write from a read.
type RequestAttributes struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	IP       string `json:"ip"`
	APIKey   bool   `json:"apiKey"`
	ReadOnly bool   `json:"readOnly"`
}

// UserAttributes carries the calling user's identity-relevant fields.
type UserAttributes struct {
	ID       string            `json:"id"`
	Username string            `json:"username"`
	Email    string            `json:"email"`
	Roles    []string          `json:"roles"`
	Groups   []string          `json:"groups"`
	Labels   map[string]string `json:"labels"`
}

// EnvAttributes describes the environment the request is for.
type EnvAttributes struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResourceAttributes describes the resource being acted on.
type ResourceAttributes struct {
	Type        string            `json:"type"` // 'container', 'stack', 'backup_plan', ...
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Owner       string            `json:"owner"`
	Image       string            `json:"image,omitempty"`
	State       string            `json:"state,omitempty"`
	Environment string            `json:"environment"`
	Labels      map[string]string `json:"labels"`
	Attrs       map[string]any    `json:"attrs"`
}

// TimeAttributes is a frozen snapshot of the current wall-clock time,
// pre-computed before the runtime starts so admins cannot read the
// clock directly through the engine.
type TimeAttributes struct {
	Year     int    `json:"year"`
	Month    int    `json:"month"`
	Day      int    `json:"day"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Weekday  int    `json:"weekday"` // 0 = Sunday
	UnixTime int    `json:"unixTime"`
	Date     string `json:"date"` // YYYY-MM-DD, UTC
}

// EvalResult is the outcome of a single policy evaluation.
type EvalResult struct {
	Matched bool   // the expression returned truthy
	Reason  string // human-readable explanation (for audit log)
	Err     error  // set when the expression failed to parse or timed out
}

// ErrABACInvalidExpression is returned when the script fails to parse.
var ErrABACInvalidExpression = errors.New("abac: invalid expression")

// ErrABACExecutionTimeout is returned when the script exceeds the budget.
var ErrABACExecutionTimeout = errors.New("abac: execution timeout")

// Evaluate runs the expression against the supplied attributes. The
// expression is wrapped in an IIFE so admins can use `return` to short
// circuit. The whole thing runs under a deadline; on timeout, the
// runtime is interrupted and an error is returned.
func (e *ABACExecutor) Evaluate(ctx context.Context, expression string, attrs *Attributes) EvalResult {
	rt := goja.New()

	// Expose fields under their JSON names. Without this goja reflects
	// the Go field names, so a documented expression like
	// resource.labels["mcharbor.tier"] reads `undefined` and throws a
	// TypeError on the index — which is how every seed policy failed.
	rt.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Pre-compute the "now" snapshot so we don't leak it as a getter,
	// and fill in any bundle the caller left nil so expressions can
	// probe absent attributes instead of throwing on them.
	attrsCopy := normalizeAttributes(attrs)

	// Whitelist of read-only helpers.
	if err := rt.Set("user", rt.ToValue(attrsCopy.User)); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	if err := rt.Set("env", rt.ToValue(attrsCopy.Env)); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	if err := rt.Set("resource", rt.ToValue(attrsCopy.Resource)); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	if err := rt.Set("now", rt.ToValue(attrsCopy.Now)); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	if err := rt.Set("request", rt.ToValue(attrsCopy.Request)); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	if err := installHelpers(rt, attrsCopy); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}

	// hasLabel(labels, key) — strict map lookup, returns "" if missing.
	if err := rt.Set("hasLabel", func(call goja.FunctionCall) goja.Value {
		m, _ := call.Argument(0).Export().(map[string]string)
		key := call.Argument(1).String()
		if v, ok := m[key]; ok {
			return rt.ToValue(v)
		}
		return rt.ToValue("")
	}); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}

	// inList(value, items...) — variadic membership.
	if err := rt.Set("inList", func(call goja.FunctionCall) goja.Value {
		target := call.Argument(0).Export()
		for i := 1; i < len(call.Arguments); i++ {
			if compareEqual(target, call.Argument(i).Export()) {
				return rt.ToValue(true)
			}
		}
		return rt.ToValue(false)
	}); err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}

	// Bound execution. The watcher goroutine calls Interrupt when the
	// deadline fires; the JS code sees an *goja.InterruptedError which
	// we wrap as ErrABACExecutionTimeout.
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		select {
		case <-execCtx.Done():
			rt.Interrupt("execution timeout")
		case <-done:
		}
	}()
	defer close(done)

	prog, err := compilePolicy(expression)
	if err != nil {
		return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
	}
	val, err := rt.RunProgram(prog)
	if err != nil {
		return classifyEvalError(err)
	}

	matched := false
	switch v := val.Export().(type) {
	case bool:
		matched = v
	case nil:
		matched = false
	default:
		// Anything truthy matches.
		matched = rt.ToValue(v).ToBoolean()
	}

	if e.logger != nil {
		e.logger.Debug("abac.evaluate", "matched", matched, "len", len(expression))
	}

	reason := "expression matched"
	if !matched {
		reason = "expression did not match"
	}
	return EvalResult{Matched: matched, Reason: reason}
}

// compilePolicy wraps the admin expression in an IIFE and compiles it.
//
// Two shapes are supported, and both are in use: a bare expression
// (`resource.labels["mcharbor.tier"] == "prod"` — what every seed
// policy and the docs use) and a statement body that returns
// explicitly. The expression form is tried first; if it does not
// compile — which is what a body containing `return` or any other
// statement does — the statement form is used instead. Wrapping a bare
// expression in a statement body alone would discard its value and
// evaluate every such policy to undefined, i.e. never match.
func compilePolicy(expression string) (*goja.Program, error) {
	expr := strings.TrimRight(strings.TrimSpace(expression), ";")
	if expr != "" {
		if prog, err := goja.Compile("policy", fmt.Sprintf("(function() { return (%s); })()", expr), false); err == nil {
			return prog, nil
		}
	}
	return goja.Compile("policy", fmt.Sprintf("(function() { %s })()", expression), false)
}

func classifyEvalError(err error) EvalResult {
	var intr *goja.InterruptedError
	if errors.As(err, &intr) {
		return EvalResult{Err: ErrABACExecutionTimeout}
	}
	return EvalResult{Err: fmt.Errorf("%w: %w", ErrABACInvalidExpression, err)}
}

// normalizeAttributes returns a copy of attrs with every bundle and
// every label map non-nil. A nil bundle reaches JS as `null`, so
// `user.labels["team"]` on an unsupplied user throws a TypeError
// instead of yielding undefined — policies must be able to evaluate
// defensively against attributes a provider could not resolve.
func normalizeAttributes(attrs *Attributes) Attributes {
	var out Attributes
	if attrs != nil {
		out = *attrs
	}
	if out.User == nil {
		out.User = &UserAttributes{}
	}
	if out.User.Labels == nil {
		out.User.Labels = map[string]string{}
	}
	if out.User.Roles == nil {
		out.User.Roles = []string{}
	}
	if out.User.Groups == nil {
		out.User.Groups = []string{}
	}
	if out.Env == nil {
		out.Env = &EnvAttributes{}
	}
	if out.Resource == nil {
		out.Resource = &ResourceAttributes{}
	}
	if out.Resource.Labels == nil {
		out.Resource.Labels = map[string]string{}
	}
	if out.Resource.Attrs == nil {
		out.Resource.Attrs = map[string]any{}
	}
	if out.Now == nil {
		out.Now = snapshotTime(time.Now())
	}
	if out.Request == nil {
		out.Request = &RequestAttributes{}
	}
	return out
}

// snapshotTime takes a Go time.Time and returns a fixed snapshot.
func snapshotTime(t time.Time) *TimeAttributes {
	wday := int(t.Weekday())
	if wday < 0 {
		wday = 0
	}
	return &TimeAttributes{
		Year:     t.Year(),
		Month:    int(t.Month()),
		Day:      t.Day(),
		Hour:     t.Hour(),
		Minute:   t.Minute(),
		Weekday:  wday,
		UnixTime: int(t.Unix()),
		Date:     t.UTC().Format("2006-01-02"),
	}
}

// compareEqual is a small helper that handles the common scalar types
// that admins are likely to throw at it. The default fallback uses
// reflect.DeepEqual so struct comparisons also work.
func compareEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
	case int:
		if bv, ok := b.(int); ok {
			return av == bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av == bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av == bv
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
