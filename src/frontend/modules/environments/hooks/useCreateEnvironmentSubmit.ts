// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  useCreateEnvironment,
  useDeployAgent,
  useCreateInstallToken,
  useProbePrerequisites,
  useInstallPrerequisites,
  type CreateEnvironmentData,
  type InstallTokenResponse,
} from './useEnvironmentActions';
import type { CreateEnvironmentFields } from './useCreateEnvironmentFields';

type Options = {
  onOpenChange: (open: boolean) => void;
  onAgentToken: (token: string) => void;
  onInstallScript?: (data: InstallTokenResponse) => void;
};

export function useCreateEnvironmentSubmit(f: CreateEnvironmentFields, opts: Options) {
  const { t } = useTranslation('environments');
  const createEnv = useCreateEnvironment();
  const deployAgent = useDeployAgent();
  const probePrereqs = useProbePrerequisites();
  const installPrereqs = useInstallPrerequisites();
  const createInstallToken = useCreateInstallToken();

  const handleCreate = () => {
    if (!f.envName.trim()) return;
    const trimmedExistingToken = f.existingAgentToken.trim();

    const data: CreateEnvironmentData = {
      name: f.envName.trim(),
      connectionType: f.connType,
    };
    if (f.connType === 'socket') {
      data.socketPath = f.socketPath;
    } else if (f.connType !== 'agent') {
      data.host = f.host;
      data.port = parseInt(f.port) || 2375;
    } else if (f.importAgentToken && trimmedExistingToken) {
      data.agentToken = trimmedExistingToken;
    }
    createEnv.mutate(data, {
      onSuccess: (resp) => {
        opts.onOpenChange(false);
        f.resetForm();
        if (!resp || typeof resp !== 'object') return;
        const envId = (resp as { environment?: { id: string } }).environment?.id;
        if ('agentToken' in resp && resp.agentToken) {
          const agentToken = resp.agentToken as string;
          if (f.deployMethod === 'ssh' && envId) {
            deployAgent.mutate({
              envId,
              data: {
                sshHost: f.sshHost,
                sshPort: parseInt(f.sshPort) || 22,
                sshUser: f.sshUser,
                hostKeyFingerprint: f.hostKeyFingerprint,
                sshAuthType: f.sshAuthType,
                sshKey: f.sshAuthType === 'key' ? f.sshKey : undefined,
                sshPassword: f.sshAuthType === 'password' ? f.sshPassword : undefined,
                rootPassword: f.rootPassword || undefined,
                method: f.sshDeployAs,
                sshSecretId: f.sshSecretId || undefined,
                saveCredentials: f.saveCredentials,
                installPrereqs: f.prereqInstall.length > 0 ? f.prereqInstall : undefined,
              },
            });
          } else if (envId) {
            createInstallToken.mutate(envId, {
              onSuccess: (tokenResp) => opts.onInstallScript?.(tokenResp),
            });
          }
          opts.onAgentToken(agentToken);
        }
      },
    });
  };

  const handleInstallPrereqs = () => {
    f.setPrereqError(undefined);
    f.setPrereqOutput(undefined);
    installPrereqs.mutate(
      { ...f.sshProbeRequest(), installPrereqs: f.prereqInstall },
      {
        onSuccess: (result) => {
          f.setPrereqOutput(result.output || undefined);
          if (!result.success) {
            f.setPrereqError(result.error ?? t('toast.agentDeployFailed'));
            if (result.prerequisites) f.setPrereqReport(result.prerequisites);
            return;
          }
          f.setPrereqReport(result.prerequisites);
          f.setPrereqInstall([]);
        },
        onError: (err: Error) => {
          f.setPrereqError(err.message || t('toast.agentDeployFailed'));
        },
      },
    );
  };

  const handleProbePrereqs = () => {
    f.setPrereqError(undefined);
    f.setPrereqOutput(undefined);
    probePrereqs.mutate(f.sshProbeRequest(), {
      onSuccess: (result) => {
        if (!result.success || !result.prerequisites) {
          f.setPrereqReport(undefined);
          f.setPrereqError(result.error ?? t('toast.agentDeployFailed'));
          return;
        }
        f.setPrereqError(undefined);
        f.setPrereqReport(result.prerequisites);
      },
      onError: (err: Error) => {
        f.setPrereqReport(undefined);
        f.setPrereqError(err.message || t('toast.agentDeployFailed'));
      },
    });
  };

  return {
    handleCreate,
    handleInstallPrereqs,
    handleProbePrereqs,
    isPending: createEnv.isPending || deployAgent.isPending,
    probing: probePrereqs.isPending,
    installing: installPrereqs.isPending,
  };
}
