// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package alerts evaluates alert rules against container, platform and backup state and delivers the resulting events.
package alerts

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

const (
	alertPollInterval   = 30 * time.Second
	alertStartupDelay   = 10 * time.Second
	alertStateRetention = 24 * time.Hour
	alertPruneInterval  = time.Hour
)

// environmentRef is one evaluation scope.
type environmentRef struct {
	ID   string
	Name string
}

// pluginRuntimeProvider yields the plugin runtime once it exists.
//
// Both halves of the module need it and neither can be handed it at
// construction time: the engine is built in main() before
// pluginruntime.Boot, and the HTTP handler is built after. Mount sets
// this once from AppDeps and everything reads through it.
var pluginRuntimeProvider func() any

// currentPluginRuntime returns the live runtime, or nil before boot.
func currentPluginRuntime() any {
	if pluginRuntimeProvider == nil {
		return nil
	}
	return pluginRuntimeProvider()
}

// hostWideScope is the single synthetic scope used for conditions that
// have no environment — backup plans, workflows, sign-ins. Evaluating
// them once per environment would multiply every alert by the number
// of hosts.
var hostWideScope = []environmentRef{{}}

// Engine evaluates enabled alert rules and delivers notifications on
// state changes.
//
// The engine knows nothing about what it is evaluating. It resolves a
// rule's source through the registry, asks it for observations, runs
// the same state machine over all of them, and fans the result out.
// Adding a condition — built-in or from a plugin — touches no code in
// this file.
type Engine struct {
	db       *sql.DB
	logger   *slog.Logger
	service  *Service
	notifier *notifier
	states   *stateStore
	history  *historyStore
	cache    *stateCache
	clock    func() time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup

	scheduleMu sync.Mutex
	lastRun    map[string]time.Time
	lastPrune  time.Time

	coordinator engineCoordinator
	nodeID      string
	hasCoord    bool

	alertNotifier notifierSlot
}

// engineCoordinator is the subset of the cluster coordinator needed by
// the alert evaluator. Keeping a local interface avoids importing the
// concrete coordinator package into this module.
type engineCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewEngine creates a background alerts engine and attaches the
// built-in sources to the shared registry.
func NewEngine(db *sql.DB, enc *encryption.Service, logger *slog.Logger, deps EngineDeps) *Engine {
	if deps.DB == nil {
		deps.DB = db
	}
	registry.attachBuiltinClients(deps)

	return &Engine{
		db:       db,
		logger:   logger,
		service:  NewService(db),
		notifier: newNotifier(db, enc, logger),
		states:   newStateStore(db),
		history:  newHistoryStore(db),
		cache:    sharedStateCache,
		lastRun:  make(map[string]time.Time),
	}
}

// NewEngineWithCoordinator creates an alerts engine that only evaluates
// rules on the elected HA leader. SQLite coordinators always report
// leader=true.
func NewEngineWithCoordinator(db *sql.DB, enc *encryption.Service, logger *slog.Logger, deps EngineDeps, coord engineCoordinator, nodeID string) *Engine {
	e := NewEngine(db, enc, logger, deps)
	e.coordinator = coord
	e.nodeID = nodeID
	e.hasCoord = coord != nil
	return e
}

// Start launches the background alert evaluation loop.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.wg.Add(1)
	go e.run(ctx)

	e.logger.Info("alerts engine started", "interval", alertPollInterval.String())
}

// Stop shuts down the background alert evaluation loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *Engine) run(ctx context.Context) {
	defer e.wg.Done()

	startup := time.NewTimer(alertStartupDelay)
	ticker := time.NewTicker(alertPollInterval)
	defer startup.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-startup.C:
			e.tick(ctx)
		case <-ticker.C:
			quiesce.Run(func() { e.tick(ctx) })
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	if !e.leaderFor(ctx) {
		// A node that is not the leader may have stale state by the time it leads again.
		e.cache.dropAll()
		e.logger.Debug("alerts engine: not leader this tick; skipping", "node", e.nodeID)
		return
	}

	e.evaluateAll(ctx)
	e.maybePrune(ctx)
}

func (e *Engine) leaderFor(ctx context.Context) bool {
	if !e.hasCoord {
		return true
	}
	return e.coordinator.LeaderOf(ctx, "alerts-engine")
}

// evaluateAll walks every enabled rule once.
func (e *Engine) evaluateAll(ctx context.Context) {
	rules, err := e.service.ListEnabled(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list enabled alerts failed", "error", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	environments, err := e.listEnvironments(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list environments failed", "error", err)
		return
	}

	for _, rule := range rules {
		client, _, ok := registry.resolve(rule.SourceType)
		if !ok {
			// The owning plugin is uninstalled, or the built-in
			// sources have not been wired yet. Either way the rule
			// stays exactly as it is — it is the operator's, not
			// ours to discard.
			e.logger.Debug("alerts engine: source unavailable", "alert", rule.ID, "source", rule.SourceType)
			continue
		}

		decl, ok := registry.condition(rule.SourceType, rule.ConditionKey)
		if !ok {
			e.logger.Warn("alerts engine: condition not declared by its source",
				"alert", rule.ID, "source", rule.SourceType, "condition", rule.ConditionKey)
			continue
		}

		if !e.due(rule, decl, time.Now()) {
			continue
		}

		scope := hostWideScope
		if decl.TargetKind != alerting.TargetNone {
			scope = e.scopeFor(rule, environments)
		}

		e.evaluateRule(ctx, rule, decl, client, scope)
	}
}

// due implements per-condition poll intervals. A registry round-trip
// for image updates and a database sweep for backup plans have no
// business running on the same 30-second cadence as a CPU gauge.
func (e *Engine) due(rule Alert, decl alerting.AlertConditionDecl, now time.Time) bool {
	interval := decl.PollInterval
	if interval <= 0 {
		interval = alertPollInterval
	}

	// A count over a window has to be sampled at least twice per
	// window, whatever the source asked for.
	//
	// The source declares an interval that suits its cost — the
	// platform source says five minutes because "a backup run failed"
	// is a state that persists until something changes it. A count is
	// not a state: each evaluation sees only the window immediately
	// behind it, so anything outside that window is invisible forever.
	//
	// Half the window, not the whole window, because evaluations are
	// quantised to the engine tick. An interval equal to the window
	// lands consecutive evaluations up to interval+tick apart while
	// each still looks back only one window, which leaves an
	// uncovered sliver between them — measured in practice as a
	// 60-second rule evaluating 80 seconds apart and missing a burst
	// by 8 seconds. Halving makes consecutive windows overlap, so
	// every instant falls inside at least one of them.
	//
	// Overlap can count one event in two consecutive evaluations.
	// That is harmless: the state machine notifies on the rising edge
	// only, so a subject already firing is not notified twice.
	if rule.Condition.WindowSeconds > 0 {
		window := time.Duration(rule.Condition.WindowSeconds) * time.Second
		if half := window / 2; half < interval {
			interval = half
		}
	}

	key := rule.ID + ":" + rule.ConditionKey

	e.scheduleMu.Lock()
	defer e.scheduleMu.Unlock()

	last, seen := e.lastRun[key]
	if seen && now.Sub(last) < interval {
		return false
	}

	e.lastRun[key] = now
	return true
}

// scopeFor resolves which environments a rule covers.
func (e *Engine) scopeFor(rule Alert, environments []environmentRef) []environmentRef {
	if len(rule.EnvironmentIDs) == 0 {
		return environments
	}

	wanted := make(map[string]struct{}, len(rule.EnvironmentIDs))
	for _, id := range rule.EnvironmentIDs {
		wanted[id] = struct{}{}
	}

	scoped := make([]environmentRef, 0, len(wanted))
	for _, env := range environments {
		if _, ok := wanted[env.ID]; ok {
			scoped = append(scoped, env)
		}
	}

	return scoped
}

// evaluateRule runs one rule across its scope.
func (e *Engine) evaluateRule(ctx context.Context, rule Alert, decl alerting.AlertConditionDecl, client alerting.AlertSourceAdapterClient, scope []environmentRef) {
	if len(scope) == 0 {
		return
	}

	stored := e.stateFor(ctx, rule.ID)

	now := e.nowUTC()
	seen := make(map[string]struct{}, len(stored)+8)

	for _, env := range scope {
		observations, err := client.Evaluate(ctx, alerting.EvaluateRequest{
			ConditionKey:    rule.ConditionKey,
			EnvironmentID:   env.ID,
			EnvironmentName: env.Name,
			WindowSeconds:   rule.Condition.WindowSeconds,
			Unit:            rule.Condition.Unit,
			Params:          rule.Condition.Params,
		})
		if err != nil {
			// A source that cannot answer leaves state untouched.
			// Clearing it here would read as "everything recovered"
			// and send a wave of resolved messages every time a
			// daemon blips.
			e.logger.Warn("alerts engine: evaluation failed",
				"alert", rule.ID, "source", rule.SourceType, "condition", rule.ConditionKey,
				"environment", env.Name, "error", err)
			continue
		}

		for _, observation := range observations {
			learnContextKeys(rule.SourceType, rule.ConditionKey, observation.Context)
			if !matchesAlertTarget(rule.Target, observation.SubjectName, observation.SubjectID) {
				continue
			}

			key := stateKey(rule.ID, env.ID, observation.SubjectID)
			seen[key] = struct{}{}

			previous := stored[key]
			previous.Key = key
			previous.AlertID = rule.ID

			result := decide(
				previous,
				conditionTriggered(observation, rule.Condition, decl),
				observation.Since,
				now,
				time.Duration(rule.Condition.ForSeconds)*time.Second,
				time.Duration(rule.CooldownSeconds)*time.Second,
				rule.NotifyOnRecovery,
			)
			result.Next.Key = key
			result.Next.AlertID = rule.ID

			if !result.Next.Active {
				// SQLite clears the acknowledgement when a state resolves; the cache must too.
				result.Next.Acknowledged = false
			}
			e.cache.put(rule.ID, result.Next)

			// Deliver before persisting: on a full disk the write fails, and the
			// alert must still reach the channels (ADR 0002).
			if result.Notify {
				e.dispatch(ctx, rule, decl, env, observation, previous, result.State, now)
			}

			if err := e.states.save(ctx, result.Next); err != nil {
				e.logger.Error("alerts engine: saving state failed", "alert", rule.ID, "error", err)
			}
		}
	}

	e.resolveMissing(ctx, rule, decl, stored, seen, now)
}

// resolveMissing handles subjects that stopped being reported.
//
// A container that was removed, a backup plan that was deleted, an
// account that stopped failing sign-ins: none of them will ever appear
// in a future observation, so without this they would stay firing
// forever.
func (e *Engine) resolveMissing(ctx context.Context, rule Alert, decl alerting.AlertConditionDecl, stored map[string]alertState, seen map[string]struct{}, now time.Time) {
	stale := make([]string, 0, 4)

	for key, previous := range stored {
		if _, ok := seen[key]; ok {
			continue
		}

		if previous.Active && previous.notified() && rule.NotifyOnRecovery {
			environment := environmentRef{ID: environmentFromStateKey(rule.ID, key)}
			observation := alerting.AlertObservation{SubjectID: subjectFromStateKey(rule.ID, key)}
			observation.SubjectName = observation.SubjectID
			e.dispatch(ctx, rule, decl, environment, observation, previous, EventStateResolved, now)
		}

		stale = append(stale, key)
	}

	if len(stale) == 0 {
		return
	}
	e.cache.forget(rule.ID, stale)
	if err := e.states.deleteKeys(ctx, stale); err != nil {
		e.logger.Error("alerts engine: clearing stale state failed", "alert", rule.ID, "error", err)
	}
}

// dispatch renders the message, delivers it, and records history.
func (e *Engine) dispatch(ctx context.Context, rule Alert, decl alerting.AlertConditionDecl, env environmentRef, observation alerting.AlertObservation, previous alertState, state string, now time.Time) {
	duration := time.Duration(0)
	if state == EventStateResolved && !previous.ActiveSince.IsZero() {
		duration = now.Sub(previous.ActiveSince)
	}

	message := renderMessage(rule.MessageTemplate, messageContext{
		AlertName:       rule.Name,
		Severity:        rule.Severity,
		State:           state,
		ConditionName:   decl.DisplayName,
		ConditionRule:   conditionDisplay(rule.Condition, decl),
		EnvironmentName: env.Name,
		SubjectName:     observation.SubjectName,
		ValueLabel:      observation.ValueLabel,
		Threshold:       formatFloat(rule.Condition.Threshold) + rule.Condition.Unit,
		Duration:        duration,
		Extra:           observation.Context,
		AlertID:         rule.ID,
		SubjectID:       observation.SubjectID,
		EnvironmentID:   env.ID,
		Operator:        rule.Condition.Operator,
		Now:             now,
	})

	input := deliveryInput{
		Title: alertTitle(rule, state), Message: message, State: state,
		StateKey:      stateKey(rule.ID, env.ID, observation.SubjectID),
		EnvironmentID: env.ID, EnvironmentName: env.Name, SubjectID: observation.SubjectID, SubjectName: observation.SubjectName,
	}
	deliveries := e.notifier.deliverChannels(ctx, rule, input)
	if fn := e.alertNotifier.Load(); fn != nil {
		(*fn)(AlertNotice{
			AlertID: rule.ID, Name: rule.Name, Title: alertTitle(rule, state), Message: message,
			Severity: normalizeSeverity(rule.Severity), State: state, SourceType: rule.SourceType,
			EnvironmentID: env.ID, SubjectID: observation.SubjectID, SubjectName: observation.SubjectName,
		})
	}

	e.notifier.deliverInApp(rule, input)

	if err := e.history.record(ctx, AlertEvent{
		AlertID:         rule.ID,
		AlertName:       rule.Name,
		SourceType:      rule.SourceType,
		ConditionKey:    rule.ConditionKey,
		EnvironmentID:   env.ID,
		EnvironmentName: env.Name,
		SubjectID:       observation.SubjectID,
		SubjectName:     observation.SubjectName,
		State:           state,
		Severity:        normalizeSeverity(rule.Severity),
		Value:           observation.Value,
		ValueLabel:      observation.ValueLabel,
		Message:         message,
		Deliveries:      deliveries,
	}); err != nil {
		e.logger.Error("alerts engine: recording history failed", "alert", rule.ID, "error", err)
	}
}

// conditionTriggered turns one observation into a boolean, according to
// the condition's kind. This is the only place the kinds diverge.
func conditionTriggered(observation alerting.AlertObservation, cfg ConditionConfig, decl alerting.AlertConditionDecl) bool {
	switch decl.Kind {
	case alerting.KindThreshold, alerting.KindCountInWindow:
		operator := cfg.Operator
		if operator == "" {
			operator = decl.DefaultOperator
		}
		if operator == "" {
			operator = ">"
		}
		threshold := cfg.Threshold
		if threshold == 0 && cfg.Operator == "" {
			threshold = decl.DefaultThreshold
		}
		return compareValue(observation.Value, operator, threshold)

	case alerting.KindDuration, alerting.KindEvent:
		return observation.Active

	default:
		return false
	}
}

func (e *Engine) maybePrune(ctx context.Context) {
	e.scheduleMu.Lock()
	if time.Since(e.lastPrune) < alertPruneInterval {
		e.scheduleMu.Unlock()
		return
	}
	e.lastPrune = time.Now()
	e.scheduleMu.Unlock()

	if err := e.states.prune(ctx, time.Now().UTC().Add(-alertStateRetention)); err != nil {
		e.logger.Warn("alerts engine: pruning states failed", "error", err)
	}
	if err := e.history.prune(ctx); err != nil {
		e.logger.Warn("alerts engine: pruning history failed", "error", err)
	}
}

func (e *Engine) listEnvironments(ctx context.Context) ([]environmentRef, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, name
		   FROM environments
		  WHERE is_active = 1
		  ORDER BY is_default DESC, name ASC
		  LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	environments := make([]environmentRef, 0, 16)
	for rows.Next() {
		var env environmentRef
		if err := rows.Scan(&env.ID, &env.Name); err != nil {
			return nil, err
		}
		environments = append(environments, env)
	}

	return environments, rows.Err()
}

// environmentFromStateKey and subjectFromStateKey split a state key
// back into its parts. The key is alertID:environmentID:subjectID and
// a subject id may itself contain colons (an image ref does), so the
// split is bounded to the two known prefixes rather than done on every
// separator.
func environmentFromStateKey(alertID, key string) string {
	rest := strings.TrimPrefix(key, alertID+":")
	index := strings.Index(rest, ":")
	if index < 0 {
		return ""
	}
	return rest[:index]
}

func subjectFromStateKey(alertID, key string) string {
	rest := strings.TrimPrefix(key, alertID+":")
	index := strings.Index(rest, ":")
	if index < 0 {
		return rest
	}
	return rest[index+1:]
}

// -----------------------------------------------------------------------------
// Delivery
// -----------------------------------------------------------------------------

// notifier fans one message out to a rule's destinations.
//
// Shared by the engine and by the test-fire endpoint so an operator's
// test travels exactly the path a real alert will.
type notifier struct {
	dispatcher *corenotify.Dispatcher
	sendInApp  func(inapp.Record) error
	logger     *slog.Logger
}

func newNotifier(db *sql.DB, enc *encryption.Service, logger *slog.Logger) *notifier {
	return &notifier{
		dispatcher: corenotify.NewDispatcher(db, enc),
		sendInApp: func(record inapp.Record) error {
			return inapp.CreateBroadcast(db, record)
		},
		logger: logger,
	}
}

// bindPluginRuntime lets plugin-owned channels resolve. The runtime
// does not exist when the module is constructed, so this is called per
// dispatch — a field assignment, cheaper than tracking whether boot
// has happened. The communications bridge binds the same way.
func (n *notifier) bindPluginRuntime(runtime any) {
	if runtime != nil && n.dispatcher != nil {
		n.dispatcher.SetPluginRuntime(runtime)
	}
}

// deliver sends to every destination whose minimum severity the rule
// meets, then to the in-app feed when the rule asks for it. Channels go first
// because the in-app feed is a SQLite write and fails on a full disk.
//
// Channels are dispatched concurrently: one slow provider must not
// delay the others, and a rule that pages three channels should not
// take three timeouts to finish.
func (n *notifier) deliver(ctx context.Context, rule Alert, in deliveryInput) []EventDelivery {
	deliveries := n.deliverChannels(ctx, rule, in)
	n.deliverInApp(rule, in)
	return deliveries
}

// deliverInApp writes the in-app feed entry when the rule asks for one.
func (n *notifier) deliverInApp(rule Alert, in deliveryInput) {
	if !rule.SendInApp {
		return
	}
	if err := n.sendInApp(inapp.Record{
		Level:      inAppLevel(rule.Severity),
		Title:      in.Title,
		Message:    in.Message,
		Action:     "alert." + in.State,
		EntityType: "alert",
		EntityID:   rule.ID,
	}); err != nil {
		n.logger.Error("alerts: in-app delivery failed", "alert", rule.ID, "error", err)
	}
}

// deliverChannels sends to the rule's communication channels only.
func (n *notifier) deliverChannels(ctx context.Context, rule Alert, in deliveryInput) []EventDelivery {
	n.bindPluginRuntime(currentPluginRuntime())
	title, message := in.Title, in.Message

	targets := make([]AlertDestination, 0, len(rule.Destinations))
	for _, destination := range rule.Destinations {
		if strings.TrimSpace(destination.ChannelID) == "" {
			continue
		}
		if !meetsMinSeverity(rule.Severity, destination.MinSeverity) {
			continue
		}
		targets = append(targets, destination)
	}

	if len(targets) == 0 {
		return []EventDelivery{}
	}

	results := make([]EventDelivery, len(targets))
	var wg sync.WaitGroup

	for i, destination := range targets {
		wg.Add(1)
		go func(index int, target AlertDestination) {
			defer wg.Done()

			result := EventDelivery{ChannelID: target.ChannelID, OK: true}
			if routed, ok := emitIncident(ctx, rule, target.ChannelID, in); routed {
				result.Mode = "incident"
				if !ok {
					result.OK, result.ErrorCode = false, "delivery_failed"
				}
				results[index] = result
				return
			}
			if _, err := n.dispatcher.SendChannel(ctx, corenotify.ChannelRequest{
				ChannelID: target.ChannelID,
				Title:     title,
				Message:   message,
			}); err != nil {
				// The provider's own error text never leaves this
				// process: a failing webhook can echo its URL, token
				// included, back in the body.
				n.logger.Error("alerts: channel delivery failed",
					"alert", rule.ID, "channel", target.ChannelID, "error", err)
				result.OK = false
				result.ErrorCode = "delivery_failed"
			}
			results[index] = result
		}(i, destination)
	}

	wg.Wait()
	return results
}

func (e *Engine) nowUTC() time.Time {
	if e.clock != nil {
		return e.clock().UTC()
	}
	return time.Now().UTC()
}

// stateFor returns a rule's firing state from memory, loading it from SQLite only
// when the rule is cold. A failed cold load proceeds with empty state: a duplicate
// alert is better than silence on a disk that cannot be read.
func (e *Engine) stateFor(ctx context.Context, alertID string) map[string]alertState {
	if states, ok := e.cache.snapshot(alertID); ok {
		return states
	}
	loaded, err := e.states.loadForAlert(ctx, alertID)
	if err != nil {
		e.logger.Error("alerts engine: loading state failed, continuing with empty state", "alert", alertID, "error", err)
		loaded = map[string]alertState{}
	}
	e.cache.seed(alertID, loaded)
	return loaded
}
