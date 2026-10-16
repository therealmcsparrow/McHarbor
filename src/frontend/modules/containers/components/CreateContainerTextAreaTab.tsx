// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconDeviceFloppy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { FieldLabel, FieldHelp } from '@resources/components/ui/Form';

type CreateContainerTextAreaTabProps = {
  label: string;
  help: string;
  placeholder: string;
  value: string;
  onChange: (v: string) => void;
  onApply: () => void;
  applyLabel: string;
  error: string | null;
  warnings: string[];
};

export function CreateContainerTextAreaTab({
  label,
  help,
  placeholder,
  value,
  onChange,
  onApply,
  applyLabel,
  error,
  warnings,
}: CreateContainerTextAreaTabProps) {
  const { t } = useTranslation('containers');
  return (
    <Box className="space-y-3">
      <FieldLabel>{label}</FieldLabel>
      <FieldHelp>{help}</FieldHelp>
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        rows={12}
        spellCheck={false}
        className="block w-full rounded-lg border border-border bg-card px-3 py-2.5 font-mono text-xs text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
      />
      {error && (
        <Box className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <span>{error}</span>
        </Box>
      )}
      {warnings.length > 0 && (
        <Box className="rounded-md border border-warning/40 bg-warning/10 p-2 text-xs text-warning">
          <BodyText weight="medium" className="mb-1">
            {t('create.headsUp')}
          </BodyText>
          <ul className="list-disc space-y-0.5 pl-4">
            {warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        </Box>
      )}
      <Flex justify="end">
        <Button variant="outline" onClick={onApply} disabled={!value.trim()}>
          <IconDeviceFloppy className="mr-2 size-4" />
          {applyLabel}
        </Button>
      </Flex>
    </Box>
  );
}
