// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlay } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import {
  USER_ICON,
  USER_ICON_CLASS,
  USER_PRESET_DEFAULT,
  USER_PRESET_OTHER,
} from '@resources/components/TerminalShellOptions';
import { useApplicableShells } from '@resources/hooks/useContainerShells';
import { PERM } from '@core/auth/permissionKeys';
import { useContainerHostSelfGate } from '../../hooks/useHostSelfGate';
import { ExecHistoryPanel, execHistoryQueryKey } from './ExecHistoryPanel';
import { ExecResultCard, type ExecResult } from './ExecResultCard';

// Preset shell paths come from a shared module so the exec tab, the
// terminal tab, and the standalone terminal page all show identical
// dropdowns. Labels for root / nobody / shell paths are translation-
// exempt per `.instructions/donottranslate.md`. The `useApplicableShells`
// hook filters this list down to the shells the running container
// actually has installed.

type ExecTabProps = {
  containerId: string;
  isRunning: boolean;
};

export function ExecTab({ containerId, isRunning }: ExecTabProps) {
  const { t } = useTranslation('containers');
  const gate = useContainerHostSelfGate(containerId);
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const [command, setCommand] = useState('');
  const { options: shellOptions, coerceShell } = useApplicableShells(containerId);
  const [shell, setShell] = useState('');
  const [userPreset, setUserPreset] = useState(USER_PRESET_DEFAULT);
  const [customUser, setCustomUser] = useState('');
  const [workDir, setWorkDir] = useState('');
  const [lastResult, setLastResult] = useState<ExecResult | null>(null);

  // Preset user identities. The empty value means "let the container's
  // own entrypoint / image default apply" — Docker passes the field
  // through only when set, so omitting it is the right default. The
  // "other" preset reveals a small custom input so users can type a
  // numeric UID, username, or uid:gid pair.
  // Four user presets per the host design:
  //   1. Container default — let the image's USER apply (sends an
  //      empty user field; Docker only sets a user when non-empty).
  //   2. root            — explicit UID 0. Marked red so operators see
  //      "this is a privileged escalation" at a glance.
  //   3. nobody          — UID 65534. Marked slate so it reads as
  //      "anonymous / unprivileged" in the dropdown.
  //   4. Custom         — free-form input below the dropdown for
  //      numeric UIDs, usernames, or uid:gid pairs (Docker accepts
  //      all three). Marked blue so the custom field stands out.
  const USER_OPTIONS = [
    {
      value: USER_PRESET_DEFAULT,
      label: t('exec.userDefault'),
      icon: USER_ICON.default,
      iconClassName: USER_ICON_CLASS.default,
    },
    {
      value: 'root',
      label: 'root',
      icon: USER_ICON.root,
      iconClassName: USER_ICON_CLASS.root,
    },
    {
      value: 'nobody',
      label: 'nobody',
      icon: USER_ICON.nobody,
      iconClassName: USER_ICON_CLASS.nobody,
    },
    {
      value: USER_PRESET_OTHER,
      label: t('exec.userOther'),
      icon: USER_ICON.other,
      iconClassName: USER_ICON_CLASS.other,
    },
  ];

  // Map the preset selector value to the literal user string we send
  // to the backend. Empty string means "let the image's USER apply"
  // (Docker's exec endpoint only sets a user when the field is non-empty).
  const resolvedUser =
    userPreset === USER_PRESET_OTHER ? customUser.trim() : userPreset === USER_PRESET_DEFAULT ? '' : userPreset;

  const run = useMutation({
    mutationFn: () => {
      const cmd = command.trim().split(/\s+/).filter(Boolean);
      if (cmd.length === 0) throw new Error(t('exec.commandRequired'));
      const params = envId ? `?env=${envId}` : '';
      return api
        .post<ExecResult>(`/containers/${containerId}/exec${params}`, {
          command: cmd,
          workDir: workDir.trim(),
          user: resolvedUser,
          shell: coerceShell(shell) || undefined,
        })
        .then((r) => r.data);
    },
    onSuccess: (result) => {
      setLastResult(result ?? null);
      void queryClient.invalidateQueries({ queryKey: execHistoryQueryKey(envId, containerId) });
    },
  });

  if (!isRunning) {
    return (
      <div className="mc-glass flex h-64 items-center justify-center rounded-lg text-muted-foreground">
        {t('processes.mustBeRunning')}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="mc-glass rounded-lg p-4">
        <div className="flex flex-wrap items-center gap-3">
          <Input
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder={t('exec.commandPlaceholder')}
            aria-label={t('exec.commandPlaceholder')}
            className="min-w-[200px] flex-1"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !run.isPending) run.mutate();
            }}
          />
          <div className="w-32">
            <Select
              value={shell}
              onChange={setShell}
              options={shellOptions}
              ariaLabel={t('terminal.shellLabel')}
              searchable={false}
            />
          </div>
          <div className="w-44">
            <Select
              value={userPreset}
              onChange={setUserPreset}
              options={USER_OPTIONS}
              ariaLabel={t('terminal.userLabel')}
              searchable={false}
            />
          </div>
          {userPreset === USER_PRESET_OTHER && (
            <Input
              value={customUser}
              onChange={(e) => setCustomUser(e.target.value)}
              placeholder={t('terminal.userPlaceholder')}
              aria-label={t('terminal.userLabel')}
              className="w-44"
            />
          )}
          <Input
            value={workDir}
            onChange={(e) => setWorkDir(e.target.value)}
            placeholder={t('exec.workDirPlaceholder')}
            aria-label={t('exec.workDirLabel')}
            className="w-40"
          />
          <Button
            onClick={() => run.mutate()}
            permission={gate.exec(PERM.CONTAINERS_MANAGE)}
            disabled={run.isPending || !command.trim()}
          >
            <IconPlayerPlay className="h-4 w-4" /> {t('exec.run')}
          </Button>
        </div>
        {run.isError && <div className="mt-3 text-xs text-destructive">{t('exec.errorRunning')}</div>}
        {run.isPending && (
          <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
            <Spinner size="sm" /> {t('exec.running')}
          </div>
        )}
      </div>

      {lastResult && <ExecResultCard lastResult={lastResult} />}

      <ExecHistoryPanel containerId={containerId} />
    </div>
  );
}
