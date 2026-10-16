// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconArrowRight } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import {
  COMPLIANCE_SECTION_LINKS,
  type ComplianceSection,
  type ComplianceSectionId,
  type ComplianceStatus,
} from '../hooks/useCompliance';
import { formatFactValue } from '../utils/complianceReport';

export const STATUS_VARIANT: Record<ComplianceStatus, 'success' | 'warning' | 'destructive' | 'info' | 'secondary'> = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
  info: 'info',
  unknown: 'secondary',
};

export function ComplianceSectionCard({ section }: { section: ComplianceSection }) {
  const { t } = useTranslation('compliance');
  const prefs = useLocaleFormat();
  const link = COMPLIANCE_SECTION_LINKS[section.id as ComplianceSectionId];

  return (
    <Card data-testid={`compliance-section-${section.id}`}>
      <CardHeader>
        <Flex align="start" justify="between" gap={3}>
          <Stack gap={1}>
            <CardTitle>{t(`sections.${section.id}.title`)}</CardTitle>
            <TypographyText tone="muted" size="sm">
              {t(`sections.${section.id}.description`)}
            </TypographyText>
          </Stack>
          <Badge variant={STATUS_VARIANT[section.status]}>{t(`status.${section.status}`)}</Badge>
        </Flex>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-1 gap-x-4 gap-y-2 sm:grid-cols-2">
          {section.facts.map((fact) => (
            <div key={fact.key} className="min-w-0">
              <dt className="text-xs text-muted-foreground">{t(`facts.${fact.key}`)}</dt>
              <dd className="text-sm break-all text-foreground">
                {formatFactValue(fact, t, (iso) => formatDate(iso, prefs))}
              </dd>
            </div>
          ))}
        </dl>
        {section.reasons.length > 0 && section.status !== 'pass' && (
          <ul className="space-y-1">
            {section.reasons.map((reason) => (
              <li key={reason} className="flex items-start gap-2 text-sm text-foreground">
                <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                {t(`reasons.${reason}`)}
              </li>
            ))}
          </ul>
        )}
        {link && section.status !== 'pass' && (
          <Button asChild variant="outline" size="sm">
            <Link to={link}>
              {t('openSettings')}
              <IconArrowRight className="size-4" aria-hidden="true" />
            </Link>
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
