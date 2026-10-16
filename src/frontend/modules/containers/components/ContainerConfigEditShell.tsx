// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file:
//   containers:configEdit.{title,description,sectionTitle,sections.environment,sections.network,sections.security,sections.resources,validationWarning}

import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SettingsShell } from '@resources/components/ui/SettingsShell';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../types/edit-form';
import type { useContainerConfig } from '../hooks/useContainerConfig';
import { ContainerConfigEditTabs, type ConfigEditTabId } from './ContainerConfigEditTabs';
import { IconRefresh } from '@tabler/icons-react';

type ContainerConfigEditShellProps = {
  container: ContainerInspect;
  /**
   * The edit lifecycle, owned by the page: Save and Cancel live in the page
   * header, so the state they act on has to be held above both.
   */
  config: ReturnType<typeof useContainerConfig>;
  /** Optional right-side header actions. */
  headerActions?: ReactNode;
  /** Whether the form should enter edit mode as soon as it is allowed. */
  autoStart?: boolean;
  /** Render in a full-page or compact (Dialog) variant. */
  variant?: 'page' | 'compact';
};

/**
 * ContainerConfigEditShell - the canonical settings-style chrome
 * around a container's edit form. It pairs `SettingsShell` with the
 * existing per-tab components; the page that mounts it owns the
 * `useContainerConfig` lifecycle and renders Save / Cancel in its header.
 */
export function ContainerConfigEditShell({
  container,
  config,
  headerActions,
  autoStart = true,
}: ContainerConfigEditShellProps) {
  const { t } = useTranslation('containers');
  const [tab, setTab] = useState<ConfigEditTabId>('environment');

  useEffect(() => {
    if (!autoStart) {
      if (config.editing) config.cancel();
      return;
    }
    if (!config.editing) config.start();
  }, [autoStart, config]);

  return (
    <SettingsShell
      title={t('configEdit.title')}
      description={t('configEdit.description', {
        name: (container.Name ?? '').replace(/^\//, ''),
      })}
      headerActions={headerActions}
      /* The page hero already names the container and this surface, so the
         shell's own title block would be the same heading twice. */
      hideHeader
      innerClassName="px-0 pb-0"
    >
      <ContainerConfigEditTabs tab={tab} onTabChange={setTab} container={container} config={config} />
    </SettingsShell>
  );
}

// Keep type-only exports tidy.
export type { EditFormData };

// Re-export for convenience so the new shell can be referenced by
// callers without an additional import.
export { IconRefresh };
