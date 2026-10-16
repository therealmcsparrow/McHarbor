// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import { api, type ApiResponse } from '@core/api/client';
import { apiErrorOf } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import type { DateRange } from '@resources/utils/date-range';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { SearchMode } from '@resources/utils/search-filter';
import { auditExportParams, AUDIT_EXPORT_FIELDS, type AuditExportFormat } from '../hooks/useAuditCompliance';
import { PERM } from '@core/auth/permissionKeys';

type AuditExportDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  action: string;
  search?: string;
  mode?: SearchMode;
  entityType: string;
  range: DateRange;
  actor?: string;
  severities?: string[];
};

export function AuditExportDialog({
  open: isOpen,
  onOpenChange,
  action,
  entityType,
  range,
  search = '',
  mode = 'contains',
  actor = '',
  severities = [],
}: AuditExportDialogProps) {
  const { t } = useTranslation('common');
  const envId = useEnvironmentStore((state) => state.currentId);
  const [format, setFormat] = useState<AuditExportFormat>('json');
  const [fields, setFields] = useState<string[]>([]);
  const [exporting, setExporting] = useState(false);

  const exportAudit = async () => {
    setExporting(true);
    try {
      const params = auditExportParams(format, action, entityType, range, envId, search, mode, {
        actor,
        severities,
        fields,
      });
      const response = await api.download('/audit/export', params);
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as ApiResponse<unknown> | null;
        throw body && body.success === false ? apiErrorOf(body) : new Error(`HTTP ${response.status}`);
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `mcharbor-audit-export.${format}`;
      link.click();
      URL.revokeObjectURL(url);
      toast.success(t('audit.exportSuccess'));
      onOpenChange(false);
    } catch (err) {
      toastError(err, t('audit.exportFailed'));
    } finally {
      setExporting(false);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-0">
        <DialogHeader>
          <DialogTitle>{t('audit.exportTitle')}</DialogTitle>
          <DialogDescription>{t('audit.exportDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Select
            value={format}
            onChange={(value) => setFormat(value as AuditExportFormat)}
            options={[
              { value: 'json', label: 'JSON' },
              { value: 'jsonl', label: 'JSONL' },
              { value: 'csv', label: 'CSV' },
            ]}
            className="w-40"
          />
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">{t('export.fields', { ns: 'audit' })}</p>
            <MultiSelect
              value={fields}
              onChange={setFields}
              options={AUDIT_EXPORT_FIELDS.map((field) => ({
                value: field,
                label: t(`export.field.${field}`, { ns: 'audit' }),
              }))}
              emptyLabel={t('export.allFields', { ns: 'audit' })}
              ariaLabel={t('export.fields', { ns: 'audit' })}
            />
            <p className="text-xs text-muted-foreground">{t('export.fieldsHint', { ns: 'audit' })}</p>
          </div>
          {(actor.trim() !== '' || severities.length > 0) && (
            <p className="text-xs text-muted-foreground">{t('export.filtersApplied', { ns: 'audit' })}</p>
          )}
          <p className="text-xs text-muted-foreground">{t('audit.exportMetadata')}</p>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={exporting}>
            {t('actions.cancel')}
          </Button>
          <Button permission={PERM.AUDIT_EXPORT} onClick={exportAudit} disabled={exporting}>
            {exporting ? <Spinner size="sm" /> : <IconDownload className="size-4" />}
            {t('audit.exportButton')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
