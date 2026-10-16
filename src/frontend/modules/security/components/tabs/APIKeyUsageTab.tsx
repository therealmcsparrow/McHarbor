// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What this key has actually been doing.
//
// The graph and the history it drills into, with a tab to themselves. On the
// previous single-page layout the table competed with the permission cards
// for the same fold; here it gets the height a request log needs.

import { useTranslation } from 'react-i18next';
import { Card, CardContent } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { APIKeyUsageChart } from '../APIKeyUsageChart';
import { APIKeyUsageEventsTable } from '../APIKeyUsageEventsTable';

type APIKeyUsageTabProps = {
  apiKeyId: string;
  /** A deleted key keeps its history; say so rather than looking broken. */
  deleted: boolean;
};

export function APIKeyUsageTab({ apiKeyId, deleted }: APIKeyUsageTabProps) {
  const { t } = useTranslation('security');

  return (
    <div className="flex min-h-0 flex-col gap-4">
      {deleted && (
        <Card>
          <CardContent className="py-3">
            <TypographyText size="xs" tone="muted">
              {t('apiKeys.detail.usageAfterDeletion')}
            </TypographyText>
          </CardContent>
        </Card>
      )}
      <APIKeyUsageChart apiKeyId={apiKeyId} />
      <div className="min-h-[32rem]">
        <APIKeyUsageEventsTable apiKeyId={apiKeyId} />
      </div>
    </div>
  );
}
