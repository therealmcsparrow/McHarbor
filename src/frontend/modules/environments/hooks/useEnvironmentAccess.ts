// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type AccessSettings = {
  configured: boolean;
  host?: string;
  port?: number;
  user?: string;
  authType?: string;
  method?: string;
  hostKeyFingerprint?: string;
  lastAt?: string;
  secretId?: string;
  secretName?: string;
  secretKind?: string;
};

type SecretOption = {
  id: string;
  name: string;
  kind: string;
};

export function useEnvironmentAccess(envId: string) {
  const { t } = useTranslation('environments');
  const queryClient = useQueryClient();

  const access = useQuery({
    queryKey: ['agent-access', envId],
    enabled: Boolean(envId),
    queryFn: () => api.get<AccessSettings>(`/agents/${envId}/access`).then(assertSuccess),
  });

  const secrets = useQuery({
    queryKey: ['agent-access-secrets', envId],
    enabled: Boolean(envId),
    queryFn: () => api.get<SecretOption[]>(`/agents/${envId}/access/secrets`).then(assertSuccess),
  });

  const [host, setHost] = useState('');
  const [port, setPort] = useState('22');
  const [user, setUser] = useState('root');
  const [authType, setAuthType] = useState('key');
  const [method, setMethod] = useState('binary');
  const [fingerprint, setFingerprint] = useState('');
  const [secretId, setSecretId] = useState('');
  const [sshKey, setSshKey] = useState('');
  const [sshPassword, setSshPassword] = useState('');

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.put(`/agents/${envId}/access`, body),
    meta: { success: t('detail.access.saved') },
    onSuccess: () => {
      setSshKey('');
      setSshPassword('');
      void queryClient.invalidateQueries({ queryKey: ['agent-access', envId] });
    },
  });

  const forget = useMutation({
    mutationFn: () => api.del(`/agents/${envId}/access`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['agent-access', envId] });
    },
  });

  const settings = access.data;
  const [syncedSettings, setSyncedSettings] = useState<typeof settings>(undefined);
  if (settings !== syncedSettings) {
    setSyncedSettings(settings);
    if (settings?.configured) {
      setHost(settings.host ?? '');
      setPort(String(settings.port ?? 22));
      setUser(settings.user ?? 'root');
      setAuthType(settings.authType ?? 'key');
      setMethod(settings.method ?? 'binary');
      setFingerprint(settings.hostKeyFingerprint ?? '');
      setSecretId(settings.secretId ?? '');
    }
  }

  const secretOptions = [
    { value: '', label: t('detail.access.secretNone') },
    ...(secrets.data ?? []).map((secret) => ({
      value: secret.id,
      label: secret.name,
    })),
  ];

  const handleSave = () =>
    save.mutate({
      sshHost: host.trim(),
      sshPort: Number(port) || 22,
      sshUser: user.trim(),
      sshAuthType: authType,
      sshKey: authType === 'key' ? sshKey : undefined,
      sshPassword: authType === 'password' ? sshPassword : undefined,
      sshSecretId: secretId || undefined,
      hostKeyFingerprint: fingerprint.trim(),
      method,
    });

  return {
    access,
    settings,
    save,
    forget,
    handleSave,
    secretOptions,
    canSave: Boolean(host.trim() && user.trim()),
    fields: {
      host,
      setHost,
      port,
      setPort,
      user,
      setUser,
      authType,
      setAuthType,
      method,
      setMethod,
      fingerprint,
      setFingerprint,
      secretId,
      setSecretId,
      sshKey,
      setSshKey,
      sshPassword,
      setSshPassword,
    },
  };
}

export type EnvironmentAccessState = ReturnType<typeof useEnvironmentAccess>;
