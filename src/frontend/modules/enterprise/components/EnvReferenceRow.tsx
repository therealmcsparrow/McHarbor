// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { TableCell, TableRow } from '@resources/components/ui/Table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { Code, Text as TypographyText } from '@resources/components/ui/Typography';
import type { EnvVariable } from '../utils/envReference';

type FlagProps = {
  label: string;
  hint: string;
  variant: 'destructive' | 'secondary' | 'warning' | 'outline';
};

function Flag({ label, hint, variant }: FlagProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant={variant} tabIndex={0}>
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{hint}</TooltipContent>
    </Tooltip>
  );
}

type RowProps = {
  variable: EnvVariable;
  copied: boolean;
  onCopy: (name: string) => void;
};

export function EnvReferenceRow({ variable, copied, onCopy }: RowProps) {
  const { t } = useTranslation('envref');
  return (
    <TableRow data-testid={`env-${variable.scope}-${variable.name}`}>
      <TableCell className="align-top">
        <div className="flex items-center gap-1">
          <Code className="break-all">{variable.name}</Code>
          <Button
            variant="ghost"
            size="icon"
            aria-label={t('copy', { name: variable.name })}
            onClick={() => onCopy(variable.name)}
          >
            {copied ? <IconCheck className="size-3.5" /> : <IconCopy className="size-3.5" />}
          </Button>
        </div>
        <TypographyText tone="muted" size="xs">
          {t(`scope.${variable.scope}`)}
        </TypographyText>
      </TableCell>
      <TableCell className="align-top">
        <Code className="break-all">{variable.defaultValue || '—'}</Code>
      </TableCell>
      <TableCell className="align-top">
        <TypographyText size="sm">{variable.description || t('noDescription')}</TypographyText>
        {variable.sources.length > 0 && (
          <TypographyText tone="muted" size="xs" className="break-all">
            {t('sources', { files: variable.sources.join(', ') })}
          </TypographyText>
        )}
      </TableCell>
      <TableCell className="align-top">
        <div className="flex flex-wrap gap-1">
          {variable.sensitive && (
            <Flag variant="destructive" label={t('flags.sensitive')} hint={t('flags.sensitiveHint')} />
          )}
          {variable.restartRequired && (
            <Flag variant="secondary" label={t('flags.restart')} hint={t('flags.restartHint')} />
          )}
          {!variable.documented && (
            <Flag variant="warning" label={t('flags.undocumented')} hint={t('flags.undocumentedHint')} />
          )}
          {!variable.inCode && <Flag variant="outline" label={t('flags.notInCode')} hint={t('flags.notInCodeHint')} />}
        </div>
      </TableCell>
    </TableRow>
  );
}
