// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Stack } from '@resources/components/ui/Layout';
import { Code, Text as TypographyText } from '@resources/components/ui/Typography';
import { copyToClipboard } from '@resources/utils/clipboard';
import { appToast } from '@resources/utils/toast';

type InstanceCardProps = {
  /**
   * Shown in full, and copyable. The instance id is a seat identifier,
   * not a credential — the licence key is — and releasing this seat from
   * the mcharbor.eu dashboard addresses it by exactly this value.
   */
  instanceId: string;
  instanceType: string;
  hostname?: string;
};

export function InstanceCard({ instanceId, instanceType, hostname }: InstanceCardProps) {
  const { t } = useTranslation('license');

  const handleCopy = () => {
    void copyToClipboard(instanceId).then(() => {
      appToast.success(t('instance.copied'));
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('instance.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <TypographyText tone="muted" size="xs" className="mb-2 block">
          {t('instance.description')}
        </TypographyText>
        <div className="flex items-center gap-2">
          <Code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2 py-1">
            {instanceId || t('instance.pending')}
          </Code>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('instance.copy')}
            title={t('instance.copy')}
            disabled={!instanceId}
            onClick={handleCopy}
          >
            <IconCopy className="size-4" />
          </Button>
        </div>

        <Stack gap={2} className="mt-3">
          <InfoRow label={t('instance.type')}>{t(`instance.type_${instanceType}`)}</InfoRow>
          {hostname && <InfoRow label={t('instance.hostname')}>{hostname}</InfoRow>}
        </Stack>
      </CardContent>
    </Card>
  );
}
