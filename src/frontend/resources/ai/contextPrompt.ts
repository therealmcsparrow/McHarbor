// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Turn the focused page context into a starter prompt.
//
// The header's AI button and the floating `AssistantFAB` used to do the
// identical thing: dispatch `openAssistant()` with no argument. Two
// controls, one behaviour — which is the definition of a duplicate.
//
// Rather than delete one (that would remove an entry point people have
// learned), the header button becomes the *context-seeded* one: on a
// container detail page it opens pre-loaded with that container's id,
// image and current state, so the first question does not have to be
// "which container are we talking about". The FAB stays the blank-page
// entry. Two controls, two behaviours.

import type { AIContext } from './registry';

type Translator = (key: string, options?: Record<string, unknown>) => string;

/** Kinds with a localised name; a forward-compatible unknown kind is shown as its raw id. */
function kindLabel(type: string, t: Translator): string {
  switch (type) {
    case 'container':
      return t('aiAssistant.seed.kind.container');
    case 'image':
      return t('aiAssistant.seed.kind.image');
    case 'stack':
      return t('aiAssistant.seed.kind.stack');
    case 'volume':
      return t('aiAssistant.seed.kind.volume');
    case 'network':
      return t('aiAssistant.seed.kind.network');
    case 'environment':
      return t('aiAssistant.seed.kind.environment');
    case 'general':
      return t('aiAssistant.seed.kind.general');
    case 'workflow':
      return t('aiAssistant.seed.kind.workflow');
    case 'log':
      return t('aiAssistant.seed.kind.log');
    case 'plugin':
      return t('aiAssistant.seed.kind.plugin');
    default:
      return type;
  }
}

/** Compact a metadata value for inclusion in a prompt. */
function renderValue(value: unknown): string | null {
  if (value === null || value === undefined || value === '') return null;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) {
    const parts = value.map(renderValue).filter(Boolean).slice(0, 5);
    return parts.length ? parts.join(', ') : null;
  }
  return null;
}

/**
 * The seed prompt for a context, or `undefined` when there is nothing
 * page-specific to say — in which case the assistant should open blank
 * rather than with a prompt that says nothing.
 */
export function contextSeedPrompt(context: AIContext, t: Translator): string | undefined {
  if (context.type === 'general' && !context.resourceName && !context.summary) {
    return undefined;
  }

  // A page that already wrote its own summary knows better than we do.
  if (context.summary) {
    return t('aiAssistant.seed.withSummary', { summary: context.summary });
  }

  const itemName = context.resourceName ?? context.resourceId;
  if (!itemName) return undefined;

  const facts: string[] = [];
  for (const [key, value] of Object.entries(context.metadata ?? {})) {
    const rendered = renderValue(value);
    // Keep the seed short — it is a starting point the user edits, not a
    // data dump. Six facts is about a line and a half.
    if (rendered && facts.length < 6) facts.push(`${key}: ${rendered}`);
  }

  return t('aiAssistant.seed.withResource', {
    kind: kindLabel(context.type, t),
    name: itemName,
    facts: facts.join(' · '),
  });
}

/** Does this context carry anything worth seeding with? */
export function hasSeedableContext(context: AIContext): boolean {
  return Boolean(context.summary || context.resourceName || context.resourceId);
}
