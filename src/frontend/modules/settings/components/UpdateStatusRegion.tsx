// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useUpdateChannels } from '@resources/hooks/useUpdateChannels';
import { isPrerelease } from '@resources/utils/versionOrder';
import { useSelfUpdateState } from '../hooks/useUpdates';

/**
 * UpdateStatusRegion is first in reading order on the McHarbor tab. It says what a
 * prerelease host with offers off can expect, and offers no way to an older version.
 */
export function UpdateStatusRegion() {
  const { t } = useTranslation('settings');
  const { settings } = useUpdateChannels();
  const { data: state } = useSelfUpdateState();
  const version = state?.currentVersion;
  if (!settings || settings.mcharborPrereleases || !isPrerelease(version)) return null;
  return (
    <section aria-label={t('prerelease.statusLabel')} className="rounded-lg border border-border bg-muted/40 p-4">
      <p className="text-sm text-foreground">{t('prerelease.mcharbor.running', { version })}</p>
    </section>
  );
}
