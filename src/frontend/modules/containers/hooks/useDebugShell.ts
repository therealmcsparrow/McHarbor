// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

export type DebugShellSession = {
  sessionId: string;
  path: string;
  arch: string;
  busyboxDigest: string;
  sizeBytes: number;
  installed: boolean;
  expiresAt: string;
};

export type InstallDebugShellVariables = {
  id: string;
};

export function useInstallDebugShell() {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id }: InstallDebugShellVariables) =>
      api.post<DebugShellSession>(`/containers/${id}/debug-shell${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: { success: t('debugShell.installed') },
  });
}

export function useDebugShellAttach(
  session: DebugShellSession | null,
  connected: boolean,
  connect: () => Promise<void> | void,
  clear: () => void,
) {
  const attachedRef = useRef('');
  const liveRef = useRef(false);

  useEffect(() => {
    if (!session || attachedRef.current === session.sessionId) return;
    attachedRef.current = session.sessionId;
    liveRef.current = false;
    void connect();
  }, [session, connect]);

  useEffect(() => {
    if (!session) return;
    if (connected) {
      liveRef.current = true;
      return;
    }
    if (liveRef.current) {
      liveRef.current = false;
      clear();
    }
  }, [session, connected, clear]);
}
