// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

// Built-in source types. These are the host's own sources; they carry
// no plugin slug, which is how the frontend tells them apart.
const (
	sourceContainer = "container"
	sourceHost      = "host"
	sourcePlatform  = "platform"
)

// sourceEntry pairs a registered spec with the code that evaluates it.
// Client is nil for a built-in whose dependencies have not been
// attached yet — the catalog is readable from process start, but a
// rule cannot be evaluated until the engine wires the Docker pools in.
type sourceEntry struct {
	spec   alerting.AlertSourceAdapterSpec
	client alerting.AlertSourceAdapterClient
}

// sourceRegistry holds every alert source the host knows about.
//
// Built-in sources are pre-registered at package init with their specs
// only, so the condition catalog is complete before main() finishes
// wiring; the engine attaches their clients once it has the Docker and
// database handles. Plugin sources are not copied in at all — they are
// resolved live through the plugin runtime, so installing or removing
// a plugin changes the catalog without a restart.
type sourceRegistry struct {
	mu      sync.RWMutex
	order   []string
	entries map[string]sourceEntry

	// pluginSource returns the plugin runtime. It is a function
	// rather than a value because the runtime does not exist yet when
	// the alerts module is wired: the loader builds host bridges
	// before pluginruntime.Boot, so a value captured at Mount time
	// would be nil forever. The communications bridge solves the same
	// ordering problem the same way.
	pluginSource func() host.AlertSourceAdapterSource
}

// registry is the process-wide alert source registry. A package-level
// singleton rather than a field on Engine because two independent
// consumers need the same view: the engine evaluates through it, and
// the HTTP handler renders the catalog and validates saved rules
// against it. Threading one instance through both would mean
// constructing the handler from the engine, which inverts the module's
// dependency direction for no benefit.
var registry = newSourceRegistry()

func newSourceRegistry() *sourceRegistry {
	r := &sourceRegistry{entries: make(map[string]sourceEntry, 8)}

	// Built-in specs are declarations only — no behaviour — so
	// registering them here is safe and keeps the catalog honest
	// before the engine exists.
	for _, spec := range []alerting.AlertSourceAdapterSpec{
		containerSourceSpec(),
		hostSourceSpec(),
		platformSourceSpec(),
	} {
		r.order = append(r.order, spec.SourceType)
		r.entries[spec.SourceType] = sourceEntry{spec: spec}
	}

	return r
}

// attachBuiltinClients gives the built-in sources the dependencies
// they need to evaluate. Called by the engine, which owns the Docker
// pools and the database handle.
func (r *sourceRegistry) attachBuiltinClients(deps EngineDeps) {
	r.mu.Lock()
	defer r.mu.Unlock()

	clients := map[string]alerting.AlertSourceAdapterClient{
		sourceContainer: newContainerSource(deps),
		sourceHost:      newHostSource(deps),
		sourcePlatform:  newPlatformSource(deps),
	}

	for sourceType, client := range clients {
		entry, ok := r.entries[sourceType]
		if !ok {
			continue
		}
		entry.client = client
		r.entries[sourceType] = entry
	}
}

// setPluginSource installs the accessor for plugin-contributed
// sources. Passing something that is not an AlertSourceAdapterSource
// (including nil) simply leaves plugin sources unavailable, which is
// the correct state for a host with no alerting plugins installed.
func (r *sourceRegistry) setPluginSource(runtime any) {
	source, ok := runtime.(host.AlertSourceAdapterSource)
	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pluginSource = func() host.AlertSourceAdapterSource { return source }
}

// setPluginSourceProvider installs a late-bound accessor. The plugin
// runtime does not exist when the alerts module mounts, so the module
// hands over a closure that reads it when it is first needed.
func (r *sourceRegistry) setPluginSourceProvider(provider func() any) {
	if provider == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.pluginSource = func() host.AlertSourceAdapterSource {
		source, ok := provider().(host.AlertSourceAdapterSource)
		if !ok {
			return nil
		}
		return source
	}
}

func (r *sourceRegistry) plugins() host.AlertSourceAdapterSource {
	r.mu.RLock()
	provider := r.pluginSource
	r.mu.RUnlock()

	if provider == nil {
		return nil
	}
	return provider()
}

// resolve returns the client that evaluates a source type.
//
// A miss is not an error condition worth shouting about: it is what a
// rule looks like while its plugin is uninstalled. Callers skip the
// rule and mark it unavailable — never delete it, because reinstalling
// the plugin has to bring the rule back exactly as it was.
func (r *sourceRegistry) resolve(sourceType string) (alerting.AlertSourceAdapterClient, alerting.AlertSourceAdapterSpec, bool) {
	r.mu.RLock()
	entry, ok := r.entries[sourceType]
	r.mu.RUnlock()

	if ok {
		if entry.client == nil {
			return nil, entry.spec, false
		}
		return entry.client, entry.spec, true
	}

	if plugins := r.plugins(); plugins != nil {
		return plugins.AlertSourceAdapter(sourceType)
	}

	return nil, alerting.AlertSourceAdapterSpec{}, false
}

// condition resolves one declaration. Used by the handler to validate
// a submitted rule and by the engine to know how to interpret an
// observation.
func (r *sourceRegistry) condition(sourceType, conditionKey string) (alerting.AlertConditionDecl, bool) {
	spec, ok := r.spec(sourceType)
	if !ok {
		return alerting.AlertConditionDecl{}, false
	}

	for _, decl := range spec.Conditions {
		if decl.Key == conditionKey {
			return decl, true
		}
	}

	return alerting.AlertConditionDecl{}, false
}

// spec returns a source's declaration set whether or not it can
// currently evaluate. The catalog and the rule card both need to
// describe a source that has no client attached.
func (r *sourceRegistry) spec(sourceType string) (alerting.AlertSourceAdapterSpec, bool) {
	r.mu.RLock()
	entry, ok := r.entries[sourceType]
	r.mu.RUnlock()

	if ok {
		return entry.spec, true
	}

	if plugins := r.plugins(); plugins != nil {
		if _, spec, found := plugins.AlertSourceAdapter(sourceType); found {
			return spec, true
		}
	}

	return alerting.AlertSourceAdapterSpec{}, false
}

// available reports whether a rule's source currently resolves. Drives
// the "source unavailable" state in the UI.
func (r *sourceRegistry) available(sourceType string) bool {
	_, ok := r.spec(sourceType)
	return ok
}

// specs returns built-in sources first, in registration order, then
// every plugin source. Built-ins lead because they are what an
// operator with no plugins installed sees, and a stable order keeps
// the condition picker from reshuffling between renders.
func (r *sourceRegistry) specs() []alerting.AlertSourceAdapterSpec {
	r.mu.RLock()
	out := make([]alerting.AlertSourceAdapterSpec, 0, len(r.order)+4)
	for _, sourceType := range r.order {
		if entry, ok := r.entries[sourceType]; ok {
			out = append(out, entry.spec)
		}
	}
	r.mu.RUnlock()

	if plugins := r.plugins(); plugins != nil {
		out = append(out, plugins.AlertSourceAdapters()...)
	}

	return out
}

// catalog renders every registered source as the API shape the rule
// builder consumes.
func (r *sourceRegistry) catalog() ConditionCatalog {
	specs := r.specs()
	sources := make([]CatalogSource, 0, len(specs))

	for _, spec := range specs {
		builtIn := strings.TrimSpace(spec.Slug) == ""

		conditions := make([]CatalogCondition, 0, len(spec.Conditions))
		for _, decl := range spec.Conditions {
			operators := decl.Operators
			if len(operators) == 0 {
				operators = defaultOperators
			}

			condition := CatalogCondition{
				Key:              decl.Key,
				DisplayName:      decl.DisplayName,
				Description:      decl.Description,
				Kind:             string(decl.Kind),
				Unit:             decl.Unit,
				Operators:        operators,
				DefaultOperator:  decl.DefaultOperator,
				DefaultThreshold: decl.DefaultThreshold,
				SupportsFor:      decl.SupportsFor,
				TargetKind:       string(decl.TargetKind),
			}

			// Built-in strings live in the host's locale files, so the
			// frontend translates them by key. A plugin owns its own
			// strings and ships them localised in DisplayName, which
			// is why it gets no key here rather than an empty lookup
			// that would render as raw dotted text.
			if builtIn {
				condition.I18nKey = "alerts.condition." + spec.SourceType + "." + decl.Key
			}

			for _, field := range decl.Fields {
				condition.Fields = append(condition.Fields, CatalogFieldDecl{
					Key:         field.Key,
					Type:        field.Type,
					Label:       field.Label,
					Required:    field.Required,
					Help:        field.Help,
					Placeholder: field.Placeholder,
				})
			}

			conditions = append(conditions, condition)
		}

		sources = append(sources, CatalogSource{
			SourceType:  spec.SourceType,
			DisplayName: spec.DisplayName,
			Description: spec.Description,
			Slug:        spec.Slug,
			Icon:        spec.Icon,
			BuiltIn:     builtIn,
			Conditions:  conditions,
		})
	}

	return ConditionCatalog{Sources: sources}
}

// Catalog returns every registered alert source and its conditions, for callers
// outside this package such as the AI tool bridge.
func Catalog() ConditionCatalog {
	return registry.catalog()
}
