// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { useHostVersion } from '@resources/hooks/useHostVersion';
import { UPDATE_CHANNELS_KEY, type UpdateChannels } from '@resources/hooks/useUpdateChannels';
import { isPrerelease } from '@resources/utils/versionOrder';

/**
 * PluginHostNote explains why an installed plugin sits on a prerelease build:
 * on a prerelease host the plugin was built for it, whatever the switches say.
 * It stays silent when a switch already explains the offer.
 */
export function PluginHostNote({ override }: { override: boolean | null | undefined }) {
  const { t } = useTranslation('plugins');
  const host = useHostVersion();
  // Reads the cache the Store header switch fills and never fetches: one observer per card would
  // refetch a failed read on every mount.
  const { data: settings } = useQuery<UpdateChannels>({ queryKey: UPDATE_CHANNELS_KEY, enabled: false });
  if (!isPrerelease(host) || override === true || settings?.pluginPrereleases === true) return null;
  return <p className="text-xs text-muted-foreground">{t('card.builtForPrereleaseHost')}</p>;
}
