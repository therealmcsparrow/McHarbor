// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CreateEnvironmentDialog } from './CreateEnvironmentDialog';
import { grantPermissions } from '../../../test/permissions';

const hookMocks = vi.hoisted(() => ({
  createMutate: vi.fn(),
  deployMutate: vi.fn(),
  installTokenMutate: vi.fn(),
}));

vi.mock('../hooks/useEnvironmentActions', () => ({
  useCreateEnvironment: () => ({ mutate: hookMocks.createMutate, isPending: false }),
  useDeployAgent: () => ({ mutate: hookMocks.deployMutate, isPending: false }),
  useCreateInstallToken: () => ({ mutate: hookMocks.installTokenMutate }),
  useProbePrerequisites: () => ({ mutate: vi.fn(), isPending: false, data: undefined, reset: vi.fn() }),
  useInstallPrerequisites: () => ({ mutate: vi.fn(), isPending: false, data: undefined, reset: vi.fn() }),
}));

vi.mock('@resources/components/ui/Select', () => ({
  Select: ({
    value,
    onChange,
    options,
    ariaLabel,
  }: {
    value: string;
    onChange: (value: string) => void;
    options: { value: string; label: string }[];
    ariaLabel?: string;
  }) => (
    <select aria-label={ariaLabel} value={value} onChange={(event) => onChange(event.target.value)}>
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
}));

vi.mock('react-i18next', () => ({
  useTranslation: (namespace: string) => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'actions.cancel': 'Cancel',
        'create.title': 'Add Environment',
        'create.description': 'Connect to a Docker host or Kubernetes cluster.',
        'create.name': 'Name',
        'create.namePlaceholderDocker': 'My Docker Host',
        'create.namePlaceholderK8s': 'My K8s Cluster',
        'create.platformLabel': 'Platform',
        'create.connectionType': 'Connection Type',
        'create.agentDescription': 'Agent description',
        'create.deployMethod': 'Deployment Method',
        'create.deployManual': 'Manual',
        'create.deploySSH': 'Automated via SSH',
        'create.deployScript': 'Install script URL',
        'create.sshHost': 'SSH Host',
        'create.sshPort': 'SSH Port',
        'create.sshUser': 'SSH User',
        'create.hostKeyFingerprint': 'SSH Host Fingerprint',
        'create.hostKeyFingerprintPlaceholder': 'SHA256:...',
        'create.hostKeyFingerprintHelp': 'Verify host key.',
        'create.sshAuthType': 'Authentication',
        'create.sshAuthKey': 'Private key',
        'create.sshAuthPassword': 'Password',
        'create.sshKey': 'SSH Private Key',
        'create.sshKeyPlaceholder': '-----BEGIN OPENSSH PRIVATE KEY-----',
        'create.sshPassword': 'SSH Password',
        'create.sshPasswordWarning': 'Password warning',
        'create.deployAs': 'Deploy As',
        'create.deployAsDocker': 'Docker container',
        'create.deployAsBinary': 'System binary',
        'create.adding': 'Adding...',
        'create.submit': 'Add Environment',
        'connectionTypes.socket': 'Unix Socket',
        'connectionTypes.tcp': 'TCP',
        'connectionTypes.tls': 'TLS',
        'connectionTypes.ssh': 'SSH',
        'connectionTypes.agent': 'Remote Agent',
        'platform.docker': 'Docker',
        'platform.kubernetes': 'Kubernetes',
      };

      return namespace === 'common' ? (translations[`actions.${key}`] ?? key) : (translations[key] ?? key);
    },
  }),
}));

describe('CreateEnvironmentDialog', () => {
  beforeEach(() => {
    grantPermissions('*');
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('sends the published agent image when deploying the agent through SSH Docker', () => {
    render(<CreateEnvironmentDialog open onOpenChange={vi.fn()} onAgentToken={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('My Docker Host'), {
      target: { value: 'Remote Docker' },
    });
    fireEvent.change(screen.getByDisplayValue('Unix Socket'), {
      target: { value: 'agent' },
    });
    fireEvent.change(screen.getByDisplayValue('Manual'), {
      target: { value: 'ssh' },
    });
    fireEvent.change(screen.getByPlaceholderText('192.168.1.100'), {
      target: { value: '192.168.1.20' },
    });
    fireEvent.change(screen.getByPlaceholderText('SHA256:...'), {
      target: { value: 'SHA256:test' },
    });
    fireEvent.change(screen.getByPlaceholderText('-----BEGIN OPENSSH PRIVATE KEY-----'), {
      target: { value: '-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Add Environment' }));

    const onSuccess = hookMocks.createMutate.mock.calls[0]?.[1]?.onSuccess;
    onSuccess({
      environment: { id: 'env_test' },
      agentToken: 'agt_test',
    });

    expect(hookMocks.deployMutate).toHaveBeenCalledWith({
      envId: 'env_test',
      data: expect.objectContaining({
        method: 'docker',
      }),
    });
    expect(hookMocks.deployMutate.mock.calls[0]?.[0]?.data?.agentImage).toBeUndefined();
  });
});
