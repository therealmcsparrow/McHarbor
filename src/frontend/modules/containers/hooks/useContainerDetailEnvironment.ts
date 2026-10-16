// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useLayoutEffect } from 'react';
import { useSearchParams } from 'react-router';
import { useEnvironmentStore } from '@resources/stores/environment';

/**
 * Keeps the store's current environment in step with the page's `?env=`
 * parameter and reports whether queries may run yet, so a deep link never
 * fetches the container from the wrong environment.
 */
export function useContainerDetailEnvironment() {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentEnvironmentId = useEnvironmentStore((state) => state.currentId);
  const environments = useEnvironmentStore((state) => state.environments);
  const setCurrentEnvironmentId = useEnvironmentStore((state) => state.setCurrentId);
  const requestedEnvironmentId = searchParams.get('env');
  const requestedEnvironmentIsValid = requestedEnvironmentId
    ? environments.length === 0 || environments.some((environment) => environment.id === requestedEnvironmentId)
    : false;
  const environmentReady =
    !requestedEnvironmentId || (requestedEnvironmentIsValid && requestedEnvironmentId === currentEnvironmentId);
  useLayoutEffect(() => {
    if (requestedEnvironmentId && environments.length > 0 && !requestedEnvironmentIsValid) {
      setSearchParams(
        (current) => {
          current.delete('env');
          return current;
        },
        { replace: true, preventScrollReset: true },
      );
      return;
    }
    if (requestedEnvironmentId && requestedEnvironmentIsValid && requestedEnvironmentId !== currentEnvironmentId) {
      setCurrentEnvironmentId(requestedEnvironmentId);
    }
  }, [
    currentEnvironmentId,
    requestedEnvironmentId,
    requestedEnvironmentIsValid,
    environments.length,
    setCurrentEnvironmentId,
    setSearchParams,
  ]);
  return { environmentReady, currentEnvironmentId };
}
