// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import { Switch } from '@resources/components/ui/Switch';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatBytes } from '@resources/utils/format';
import type { CheckFamily, CheckFinding, CheckSeverity } from '../types';

const FAMILIES: CheckFamily[] = ['integrity', 'orphans', 'schema', 'bloat'];

function severityVariant(severity: CheckSeverity) {
  if (severity === 'fail') return 'destructive';
  if (severity === 'warn') return 'warning';
  if (severity === 'ok') return 'success';
  if (severity === 'info') return 'info';
  return 'secondary';
}

type Props = {
  findings: CheckFinding[];
  selected: Set<string>;
  onToggle: (id: string) => void;
};

export function DatabaseFindingsTable({ findings, selected, onToggle }: Props) {
  const { t } = useTranslation('database');

  return (
    <Stack gap={5}>
      {FAMILIES.map((family) => {
        const rows = findings.filter((f) => f.family === family);
        if (rows.length === 0) return null;

        return (
          <Stack key={family} gap={2}>
            <TypographyText weight="medium">{t(`check.family.${family}`)}</TypographyText>
            <Stack gap={1}>
              {rows.map((finding) => (
                <Flex
                  key={finding.id}
                  align="center"
                  justify="between"
                  gap={3}
                  className="rounded-lg border border-border bg-muted/20 p-3"
                >
                  <Stack gap={1} className="min-w-0">
                    <TypographyText size="sm">
                      {t(`check.finding.${finding.key}`, {
                        table: finding.table ?? '',
                        column: finding.column ?? '',
                        refTable: finding.refTable ?? '',
                        refColumn: finding.refColumn ?? '',
                        index: finding.index ?? '',
                        count: finding.count ?? 0,
                        size: formatBytes(finding.bytes ?? 0),
                      })}
                    </TypographyText>
                    {finding.reason && (
                      <TypographyText size="xs" tone="muted">
                        {t(`check.reason.${finding.reason}`)}
                      </TypographyText>
                    )}
                  </Stack>

                  <Flex align="center" gap={2} className="shrink-0">
                    <Badge variant={severityVariant(finding.severity)}>{t(`check.severity.${finding.severity}`)}</Badge>
                    <StatusError error={finding.detail} />
                    {finding.remedy && (
                      <Switch
                        checked={selected.has(finding.id)}
                        onCheckedChange={() => onToggle(finding.id)}
                        aria-label={t(`check.remedy.${finding.remedy}`)}
                      />
                    )}
                  </Flex>
                </Flex>
              ))}
            </Stack>
          </Stack>
        );
      })}
    </Stack>
  );
}
