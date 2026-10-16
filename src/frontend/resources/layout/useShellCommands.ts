// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconInfoCircle, IconLogout, IconRefresh, IconSparkles, IconUserCircle } from '@tabler/icons-react';
import { openAssistant } from '@resources/ai/openAssistant';
import { useCommandRegistry, type CommandEntry } from '@resources/hooks/useCommandRegistry';

type ShellCommandsArgs = {
  canRestartMcHarbor: boolean;
  logout: () => void;
  onRestart: () => void;
};

export function useShellCommands({ canRestartMcHarbor, logout, onRestart }: ShellCommandsArgs) {
  const { t } = useTranslation('common');
  const { t: tAI } = useTranslation('ai');
  const shellCommands = useMemo<CommandEntry[]>(() => {
    const commands: CommandEntry[] = [
      {
        id: 'shell:assistant:open',
        label: tAI('aiAssistant.menuItem'),
        icon: IconSparkles,
        group: 'actions',
        to: '',
        action: () => openAssistant(),
        keywords: ['ai', 'assistant', 'chat', 'help'],
        priority: 60,
      },
      {
        id: 'shell:profile:open',
        label: t('profile.menuItem'),
        icon: IconUserCircle,
        group: 'navigation',
        to: '/profile',
        keywords: ['user', 'account', 'profile'],
        priority: 20,
      },
      {
        id: 'shell:about:open',
        label: t('about.menuItem'),
        icon: IconInfoCircle,
        group: 'navigation',
        to: '/settings?tab=about',
        keywords: ['about', 'version', 'settings'],
        priority: 10,
      },
      {
        id: 'shell:auth:signout',
        label: t('actions.signOut'),
        icon: IconLogout,
        group: 'actions',
        to: '',
        action: logout,
        keywords: ['logout', 'log out', 'sign out'],
        priority: 5,
      },
    ];

    if (canRestartMcHarbor) {
      commands.push({
        id: 'shell:system:restart',
        label: t('system.menu.restartMcHarbor'),
        icon: IconRefresh,
        group: 'actions',
        to: '',
        action: onRestart,
        keywords: ['restart', 'mcharbor', 'system'],
        priority: 50,
      });
    }

    return commands;
  }, [canRestartMcHarbor, logout, onRestart, t, tAI]);
  useCommandRegistry(shellCommands);
}
