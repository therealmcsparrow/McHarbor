// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * AI Context & Capability Registry
 * =================================
 *
 * This is the single source of truth for what the AI assistant knows about
 * the user's current state, and what actions it can perform. Any page or
 * component in McHarbor (including future addons / plugins) can register
 * context + capabilities via the hooks exported here. The AI dialog
 * automatically consumes them.
 *
 * Three concepts:
 *
 *   1. **AIContext**     — what the assistant should know about right now
 *                          (e.g. the focused container, the selected env)
 *   2. **AICapability**  — an action the assistant can perform
 *                          (e.g. restart a container, open a page)
 *   3. **AIProviderInfo** — extension metadata (plugin name, version)
 *
 * Usage from a page:
 *
 * ```tsx
 * useAIContext({
 *   source: 'container-detail',
 *   type: 'container',
 *   resourceId: id,
 *   resourceName: name,
 *   metadata: { state, image },
 *   capabilities: [
 *     {
 *       id: 'restart_container',
 *       label: 'Restart container',
 *       danger: 'medium',
 *       run: () => mutateRestart(id),
 *     },
 *   ],
 * });
 * ```
 *
 * The hook auto-cleans on unmount, so navigating away from a page drops
 * its context cleanly.
 */

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

/** Resource kinds the assistant can talk about. Add new types as new modules ship. */
export type AIResourceKind =
  | 'general'
  | 'environment'
  | 'container'
  | 'image'
  | 'volume'
  | 'network'
  | 'stack'
  | 'workflow'
  | 'log'
  | 'plugin'
  | (string & {}); // forward-compatible: unknown kinds render as a generic block

/** Danger level of a capability, used for confirmation prompts in the UI. */
export type AICapabilityDanger = 'none' | 'low' | 'medium' | 'high';

/** Definition of a single thing the AI assistant can perform. */
export interface AICapability {
  /** Stable id used by the registry. Must be unique within the page. */
  id: string;
  /** Human-readable button label (e.g. "Restart container"). */
  label: string;
  /** Optional longer description shown in tooltips. */
  description?: string;
  /** Visual variant for the action button. */
  variant?: 'default' | 'outline' | 'destructive';
  /** Icon name from @tabler/icons-react, looked up at render time. */
  icon?: string;
  /** Danger level; controls whether the UI asks for confirmation. */
  danger?: AICapabilityDanger;
  /** Function called when the user clicks the action chip. */
  run: () => void | Promise<void>;
  /** Optional JSON-shape hint for the LLM (OpenAI tool-calling schema). */
  parameters?: Record<string, unknown>;
}

/** A single page-registered context block. */
export interface AIContext {
  /** Stable id of the contributor (e.g. 'container-detail', 'addon:myaddon:page'). */
  source: string;
  /** Resource kind for the LLM. */
  type: AIResourceKind;
  /** Resource id (e.g. container id, image id). */
  resourceId?: string;
  /** Resource display name (e.g. "nginx-proxy"). */
  resourceName?: string;
  /** Arbitrary structured metadata — the LLM uses this for grounding. */
  metadata?: Record<string, unknown>;
  /** Pre-rendered natural-language summary (optional convenience for the LLM). */
  summary?: string;
  /** Capabilities this page exposes to the assistant. */
  capabilities?: AICapability[];
  /** Priority when multiple contexts are active; higher wins for the focused summary. */
  priority?: number;
}

/** Provider (extension) metadata so the AI settings dashboard can list them. */
export interface AIProviderInfo {
  id: string;
  name: string;
  version?: string;
  description?: string;
}

interface AIContextStore {
  contexts: AIContext[];
  providers: AIProviderInfo[];
}

type Listener = () => void;

class AIContextRegistry {
  private store: AIContextStore = { contexts: [], providers: [] };
  private listeners = new Set<Listener>();

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify() {
    this.listeners.forEach((l) => l());
  }

  getSnapshot(): AIContextStore {
    return this.store;
  }

  /** Register a context. Returns an unregister function. */
  register(context: AIContext): () => void {
    const idx = this.store.contexts.findIndex((c) => c.source === context.source);
    if (idx >= 0) {
      this.store.contexts[idx] = context;
    } else {
      this.store.contexts = [...this.store.contexts, context];
    }
    this.notify();
    return () => {
      this.store.contexts = this.store.contexts.filter((c) => c.source !== context.source);
      this.notify();
    };
  }

  /** Register or update a provider (addon / extension) descriptor. */
  registerProvider(provider: AIProviderInfo): () => void {
    const idx = this.store.providers.findIndex((p) => p.id === provider.id);
    if (idx >= 0) {
      this.store.providers[idx] = provider;
    } else {
      this.store.providers = [...this.store.providers, provider];
    }
    this.notify();
    return () => {
      this.store.providers = this.store.providers.filter((p) => p.id !== provider.id);
      this.notify();
    };
  }
}

const registry = new AIContextRegistry();

/**
 * Selects the focused context: the highest-priority context, falling back
 * to "general" when none are registered.
 */
export function pickFocusedContext(contexts: AIContext[]): AIContext {
  if (contexts.length === 0) {
    return { source: 'system', type: 'general' };
  }
  const sorted = [...contexts].sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0));
  return sorted[0] ?? { source: 'system', type: 'general' };
}

const AIContextStateContext = createContext<{
  store: AIContextStore;
  subscribe: (l: Listener) => () => void;
} | null>(null);

interface AIContextProviderProps {
  children: ReactNode;
}

/**
 * Mounts the AI context registry at the root of the app. Wrap the entire
 * authenticated tree once (already done in AppLayout).
 */
export function AIContextProvider({ children }: AIContextProviderProps) {
  const [store, setStore] = useState<AIContextStore>(registry.getSnapshot());

  useEffect(() => {
    return registry.subscribe(() => setStore(registry.getSnapshot()));
  }, []);

  const value = useMemo(() => ({ store, subscribe: registry.subscribe.bind(registry) }), [store]);

  return <AIContextStateContext.Provider value={value}>{children}</AIContextStateContext.Provider>;
}

interface UseAIContextOptions {
  /** Disable the registration entirely (e.g. while loading). */
  enabled?: boolean;
}

/**
 * Hook for any component to publish its current AI context.
 * The registration is cleaned up automatically when the component unmounts
 * or when the inputs change.
 */
export function useAIContext(context: AIContext | null | undefined, options: UseAIContextOptions = {}): void {
  const enabled = options.enabled ?? true;

  useEffect(() => {
    if (!enabled || !context) return;
    return registry.register(context);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, context?.source, context?.type, context?.resourceId, context?.resourceName]);
}

/**
 * Hook for plugins / addons to advertise their presence to the AI
 * settings dashboard.
 */
export function useAIProvider(provider: AIProviderInfo | null | undefined): void {
  useEffect(() => {
    if (!provider) return;
    return registry.registerProvider(provider);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [provider?.id, provider?.name, provider?.version]);
}

/**
 * Hook to read all active contexts (sorted by priority) and the merged
 * focused context. The AI dialog uses this to build its prompt.
 */
export function useAIContexts(): {
  contexts: AIContext[];
  providers: AIProviderInfo[];
  focused: AIContext;
  registerContext: (ctx: AIContext) => () => void;
  registerProvider: (p: AIProviderInfo) => () => void;
} {
  const ctx = useContext(AIContextStateContext);
  const force = useRef(0);
  const [, forceRender] = useState(0);

  useEffect(() => {
    if (!ctx) return;
    return ctx.subscribe(() => forceRender(force.current++));
  }, [ctx]);

  const store = ctx?.store ?? { contexts: [], providers: [] };
  const focused = useMemo(() => pickFocusedContext(store.contexts), [store.contexts]);

  const registerContext = useCallback((c: AIContext) => registry.register(c), []);
  const registerProvider = useCallback((p: AIProviderInfo) => registry.registerProvider(p), []);

  return {
    contexts: store.contexts,
    providers: store.providers,
    focused,
    registerContext,
    registerProvider,
  };
}

/**
 * Lower-level: imperatively register a context from non-React code
 * (e.g. inside event handlers). Returns an unregister function.
 */
export function registerAIContext(ctx: AIContext): () => void {
  return registry.register(ctx);
}

export function registerAIProvider(provider: AIProviderInfo): () => void {
  return registry.registerProvider(provider);
}
