// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { ComplianceStatus } from '../hooks/useCompliance';
import { groupControls, type PackControl, type PackResult } from '../hooks/useCompliancePacks';
import { STATUS_VARIANT } from './ComplianceSectionCard';

type CompliancePackControlsProps = {
  controls: PackControl[];
  results: PackResult[];
};

export function CompliancePackControls({ controls, results }: CompliancePackControlsProps) {
  const { t } = useTranslation('compliancepacks');
  const byId = new Map(results.map((result) => [result.id, result]));

  if (controls.length === 0) return null;

  return (
    <Stack gap={4}>
      {groupControls(controls).map(([group, groupedControls]) => (
        <Card key={group} data-testid={`compliance-pack-group-${group}`}>
          <CardHeader>
            <CardTitle>{t(`group.${group}`)}</CardTitle>
          </CardHeader>
          <CardContent>
            <Stack gap={2}>
              {groupedControls.map((control) => (
                <ControlRow key={control.id} control={control} result={byId.get(control.id)} />
              ))}
            </Stack>
          </CardContent>
        </Card>
      ))}
    </Stack>
  );
}

function ControlRow({ control, result }: { control: PackControl; result?: PackResult }) {
  const { t } = useTranslation('compliancepacks');
  const [isOpen, setOpen] = useState(false);
  const statusValue: ComplianceStatus | undefined = result?.status;
  const facts = result?.facts.filter((fact) => fact.value !== '') ?? [];
  const reasons = result?.reasons ?? [];
  const expandable = facts.length > 0 || reasons.length > 0;

  return (
    <div className="rounded-md border border-border p-3" data-testid={`compliance-pack-control-${control.id}`}>
      <Flex align="start" justify="between" gap={3} wrap>
        <Stack gap={1} className="min-w-[16rem] flex-1">
          <TypographyText size="sm">
            <span className="font-mono">{control.id}</span> {control.title}
          </TypographyText>
          <TypographyText tone="muted" size="sm">
            {control.requirement}
          </TypographyText>
        </Stack>
        <Flex align="center" gap={2} wrap>
          <Badge variant="secondary">{t(`coverage.${control.coverage}`)}</Badge>
          {statusValue ? <Badge variant={STATUS_VARIANT[statusValue]}>{t(`verdict.${statusValue}`)}</Badge> : null}
          {expandable ? (
            <Button
              variant="ghost"
              size="icon"
              aria-label={isOpen ? t('actions.hideFacts') : t('actions.showFacts')}
              onClick={() => setOpen((current) => !current)}
            >
              {isOpen ? (
                <IconChevronDown className="size-4" aria-hidden="true" />
              ) : (
                <IconChevronRight className="size-4" aria-hidden="true" />
              )}
            </Button>
          ) : null}
        </Flex>
      </Flex>
      {isOpen ? (
        <Stack gap={2} className="mt-3 border-t border-border pt-3">
          {reasons.map((reason) => (
            <TypographyText key={reason} tone="muted" size="sm">
              {t(`reason.${reason}`)}
            </TypographyText>
          ))}
          {facts.map((fact) => (
            <Flex key={fact.key} align="center" justify="between" gap={3}>
              <TypographyText tone="muted" size="sm">
                {t(`fact.${fact.key}`)}
              </TypographyText>
              <TypographyText size="sm" className="font-mono">
                {fact.value}
              </TypographyText>
            </Flex>
          ))}
        </Stack>
      ) : null}
    </div>
  );
}
