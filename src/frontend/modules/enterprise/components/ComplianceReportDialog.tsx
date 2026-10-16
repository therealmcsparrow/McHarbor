// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { IconDownload } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { COMPLIANCE_SECTIONS, fetchComplianceReport, type ComplianceSection } from '../hooks/useCompliance';
import {
  buildComplianceDocument,
  downloadBlob,
  redactionPreview,
  reportFileName,
  type ComplianceExportFormat,
} from '../utils/complianceReport';
import { renderComplianceDocx, renderCompliancePdf } from '../utils/complianceExport';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sections: ComplianceSection[];
};

export function ComplianceReportDialog({ open: isOpen, onOpenChange, sections }: Props) {
  const { t } = useTranslation('compliance');
  const prefs = useLocaleFormat();
  const [selected, setSelected] = useState<string[]>([...COMPLIANCE_SECTIONS]);
  const [format, setFormat] = useState<ComplianceExportFormat>('json');
  const [redact, setRedact] = useState(true);

  const download = useMutation({
    mutationFn: async () => {
      const report = await fetchComplianceReport({ sections: selected, redact, export: true });
      if (format === 'json') {
        const json = new Blob([`${JSON.stringify(report, null, 2)}\n`], { type: 'application/json' });
        downloadBlob(json, reportFileName(report, 'json'));
        return;
      }
      const modelValue = buildComplianceDocument(report, t, (iso) => formatDate(iso, prefs));
      const blob = format === 'docx' ? await renderComplianceDocx(modelValue) : await renderCompliancePdf(modelValue);
      downloadBlob(blob, reportFileName(report, format));
    },
    onSuccess: () => onOpenChange(false),
  });

  const toggle = (id: string, on: boolean) =>
    setSelected((cur) =>
      on ? COMPLIANCE_SECTIONS.filter((s) => s === id || cur.includes(s)) : cur.filter((s) => s !== id),
    );
  const preview = redactionPreview(sections, selected);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl p-0">
        <DialogHeader>
          <DialogTitle>{t('report.title')}</DialogTitle>
          <DialogDescription>{t('report.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium text-foreground">{t('report.sections')}</legend>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {COMPLIANCE_SECTIONS.map((id) => (
                <div key={id} className="flex items-center gap-2">
                  <Checkbox
                    id={`compliance-section-${id}`}
                    checked={selected.includes(id)}
                    onCheckedChange={(v) => toggle(id, v === true)}
                  />
                  <Label htmlFor={`compliance-section-${id}`}>{t(`sections.${id}.title`)}</Label>
                </div>
              ))}
            </div>
            <Button variant="ghost" size="sm" onClick={() => setSelected([...COMPLIANCE_SECTIONS])}>
              {t('report.selectAll')}
            </Button>
          </fieldset>
          <div className="space-y-1">
            <Label>{t('report.format')}</Label>
            <Select
              value={format}
              onChange={(v) => setFormat(v as ComplianceExportFormat)}
              ariaLabel={t('report.format')}
              searchable={false}
              options={[
                { value: 'json', label: t('report.formatJson') },
                { value: 'docx', label: t('report.formatDocx') },
                { value: 'pdf', label: t('report.formatPdf') },
              ]}
              className="w-56"
            />
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Switch id="compliance-redact" checked={redact} onCheckedChange={setRedact} />
              <Label htmlFor="compliance-redact">{t('report.redact')}</Label>
            </div>
            <TypographyText tone="muted" size="sm">
              {t('report.redactHint')}
            </TypographyText>
            {redact && (
              <div className="rounded-md border border-border bg-muted/30 p-3">
                <TypographyText size="sm" weight="semibold">
                  {t('report.preview')}
                </TypographyText>
                {preview.length === 0 ? (
                  <TypographyText tone="muted" size="sm">
                    {t('report.previewNone')}
                  </TypographyText>
                ) : (
                  <ul className="mt-1 list-disc pl-5 text-sm text-foreground" data-testid="redaction-preview">
                    {preview.map((f) => (
                      <li key={f.key}>{t(`facts.${f.key}`)}</li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>
          {selected.length === 0 && <Alert variant="warning">{t('report.noSections')}</Alert>}
          {download.error && (
            <Alert variant="destructive" title={t('report.failed')}>
              {download.error.message}
            </Alert>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={download.isPending}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.COMPLIANCE_VIEW}
            onClick={() => download.mutate()}
            disabled={download.isPending || selected.length === 0}
          >
            {download.isPending ? <Spinner size="sm" /> : <IconDownload className="size-4" aria-hidden="true" />}
            {t('report.download')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
