// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Public entry point for the AI assistant subsystem. Import from any
 * module (built-in or third-party addon) to:
 *
 *   1. Publish the current page's AI context
 *   2. Advertise your addon's AI capabilities
 *   3. Read what other parts of McHarbor have registered
 *
 * Example from a plugin:
 *
 * ```tsx
 * import { useAIContext, useAIProvider } from '@resources/ai';
 *
 * useAIProvider({
 *   id: 'my-plugin',
 *   name: 'Awesome Plugin',
 *   version: '1.2.3',
 *   description: 'AI-aware container scheduling',
 * });
 *
 * useAIContext({
 *   source: 'my-plugin:scheduler',
 *   type: 'general',
 *   metadata: { queueDepth: queue.length },
 *   capabilities: [
 *     {
 *       id: 'rebalance',
 *       label: 'Rebalance workload',
 *       run: () => rebalance(),
 *     },
 *   ],
 * });
 * ```
 */

export type { AIContext, AIResourceKind, AICapability, AICapabilityDanger, AIProviderInfo } from './registry';

export {
  AIContextProvider,
  useAIContext,
  useAIProvider,
  useAIContexts,
  registerAIContext,
  registerAIProvider,
  pickFocusedContext,
} from './registry';

export { OPEN_AI_ASSISTANT_EVENT, openAssistant, type OpenAIAssistantDetail } from './openAssistant';

export { AssistantFAB } from './AssistantFAB';

export { contextSeedPrompt, hasSeedableContext } from './contextPrompt';
