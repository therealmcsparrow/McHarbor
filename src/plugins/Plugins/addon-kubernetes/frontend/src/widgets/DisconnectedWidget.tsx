// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCloudOff } from '@tabler/icons-react';
import { WidgetEmpty } from '@modules/dashboard/components/widget-kit';
import type { SummaryReason } from '../types';

function unhandledReason(reason: never): never {
  throw new Error(`unhandled summary reason: ${String(reason)}`);
}

/**
 * The empty state every widget shows when the summary came back disconnected.
 * The switch is exhaustive on purpose: a new `SummaryReason` stops type-checking
 * here instead of quietly rendering as "No cluster connected".
 */
type DisconnectedWidgetProps = { reason: SummaryReason | undefined; withHint?: boolean };

export function DisconnectedWidget({ reason, withHint = true }: DisconnectedWidgetProps) {
  const { t } = useTranslation('kubernetes');
  const icon = <IconCloudOff className="size-5" aria-hidden="true" />;
  const openHint = withHint ? t('widgets.common.openHint') : undefined;

  switch (reason) {
    case 'unreachable':
      return <WidgetEmpty tone="danger" icon={icon} title={t('widgets.common.unreachable')} hint={openHint} />;
    case 'credentials_refused':
      return (
        <WidgetEmpty
          tone="danger"
          icon={icon}
          title={t('widgets.common.credentialsRefused')}
          hint={withHint ? t('errors.reason.credentials_refused') : undefined}
        />
      );
    case 'api_unverified':
      return (
        <WidgetEmpty
          tone="danger"
          icon={icon}
          title={t('widgets.common.apiUnverified')}
          hint={withHint ? t('errors.reason.api_unverified') : undefined}
        />
      );
    case 'no_environment':
    case 'not_kubernetes':
    case undefined:
      return <WidgetEmpty tone="neutral" icon={icon} title={t('widgets.common.notConfigured')} hint={openHint} />;
    default:
      return unhandledReason(reason);
  }
}
