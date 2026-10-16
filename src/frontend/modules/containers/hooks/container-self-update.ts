// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { api } from '@core/api/client';
import type { BatchProgressContext } from '@resources/hooks/useBatchProgressOperation';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ContainerOperationTarget = {
  id: string;
  name: string;
  image: string;
};

const selfUpdateRecoveryTimeoutMs = 60_000;
const selfUpdatePollIntervalMs = 2_000;

export function isSelfUpdateTarget(target: ContainerOperationTarget) {
  const normalizedName = target.name.replace(/^\//, '').toLowerCase();
  const normalizedImage = target.image.toLowerCase();

  return normalizedName === 'mcharbor' || normalizedImage.includes('/mcharbor');
}

async function delay(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

export async function waitForSelfUpdateRecovery(
  target: ContainerOperationTarget,
  envQuery: string,
  t: (key: string, options?: Record<string, unknown>) => string,
  log?: BatchProgressContext['log'],
) {
  const deadline = Date.now() + selfUpdateRecoveryTimeoutMs;
  const waitingMessage = t('operations.log.waitingForApiRecovery');
  log?.(waitingMessage, {
    detail: waitingMessage,
  });

  while (Date.now() < deadline) {
    try {
      const health = await fetch('/api/health', { credentials: 'include' });
      if (health.ok) {
        const containers = await api
          .get<
            Array<{
              Id?: string;
              ID?: string;
              Names?: string[];
              Image?: string;
            }>
          >(`/containers${envQuery}${envQuery ? '&' : '?'}all=true`)
          .then(assertSuccess);

        const match = containers.find((container) => {
          const names = container.Names ?? [];
          const byName = names.some(
            (existingName) =>
              existingName.replace(/^\//, '').toLowerCase() === target.name.replace(/^\//, '').toLowerCase(),
          );
          const byImage = (container.Image ?? '').toLowerCase() === target.image.toLowerCase();
          return byName || byImage;
        });

        return match?.Id ?? match?.ID ?? target.id;
      }
    } catch {
      // The app is expected to be unavailable while the container restarts.
    }

    await delay(selfUpdatePollIntervalMs);
  }

  throw new Error(t('operations.log.apiRecoveryTimeout'));
}
