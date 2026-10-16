// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// --- Agent Deploy ---

export type DeployRequest = {
  sshHost: string;
  sshPort: number;
  sshUser: string;
  sshAuthType: 'key' | 'password';
  sshKey?: string;
  sshPassword?: string;
  rootPassword?: string;
  sshSecretId?: string;
  saveCredentials?: boolean;
  hostKeyFingerprint: string;
  method: 'docker' | 'binary';
  agentImage?: string;
  installPrereqs?: string[];
  probeOnly?: boolean;
  scope?: 'docker' | 'vm';
};

export type Prerequisite = {
  key: string;
  label: string;
  present: boolean;
  detail?: string;
  remediation?: string;
  required: boolean;
};

export type Escalation = 'none' | 'sudo' | 'sudo_password' | 'su' | 'unavailable';

export type PrerequisiteReport = {
  os: string;
  arch: string;
  distro?: string;
  items: Prerequisite[];
  missing?: string[];
  escalation?: Escalation;
  needsRootPassword?: boolean;
  scope?: 'docker' | 'vm';
};

export type SavedDeployment = {
  host: string;
  port: number;
  user: string;
  authType: 'key' | 'password';
  secretId: string;
  hostKeyFingerprint: string;
  method: 'docker' | 'binary';
  lastAt?: string;
};

export type DeployResult = {
  success: boolean;
  output?: string;
  error?: string;
  code?: string;
  os?: string;
  arch?: string;
  prerequisites?: PrerequisiteReport;
  credentialSaved?: boolean;
};

export function useDeployAgent() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: ({ envId, data }: { envId: string; data: DeployRequest }) =>
      api
        .post<DeployResult>(`/agents/${envId}/deploy`, data)
        .then(assertSuccess)
        .then((result) => {
          if (!result.success) {
            throw new Error(result.error ?? t('toast.agentDeployFailed'));
          }
          return result;
        }),
    meta: { success: t('toast.agentDeployed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

export function useProbePrerequisites() {
  return useMutation({
    mutationFn: (data: DeployRequest) => api.post<DeployResult>(`/agents/probe`, data).then(assertSuccess),
  });
}

export function useInstallPrerequisites() {
  return useMutation({
    mutationFn: (data: DeployRequest) => api.post<DeployResult>(`/agents/install-prereqs`, data).then(assertSuccess),
  });
}

export function useRedeployAgent() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('environments');
  return useMutation({
    mutationFn: ({ envId, installPrereqs }: { envId: string; installPrereqs?: string[] }) =>
      api
        .post<DeployResult>(`/agents/${envId}/redeploy`, { installPrereqs })
        .then(assertSuccess)
        .then((result) => {
          if (!result.success) {
            throw new Error(result.error ?? t('toast.agentDeployFailed'));
          }
          return result;
        }),
    meta: { success: t('toast.agentDeployed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}
