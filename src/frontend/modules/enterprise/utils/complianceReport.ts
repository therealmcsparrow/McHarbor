// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import type { ComplianceFact, ComplianceReport, ComplianceSection, ComplianceStatus } from '../hooks/useCompliance';

export type ComplianceT = TFunction<'compliance'>;

const REDACTED = '[redacted]';

export function formatFactValue(fact: ComplianceFact, t: ComplianceT, formatDate: (iso: string) => string): string {
  if (fact.value === REDACTED) return REDACTED;
  if (fact.value === '') return t('values.none');
  switch (fact.kind) {
    case 'bool':
      return fact.value === 'true' ? t('values.yes') : t('values.no');
    case 'percent':
      return t('values.percent', { value: fact.value });
    case 'days': {
      const n = Number(fact.value);
      if (n === 0 && fact.key !== 'daysRemaining') return t('values.noLimit');
      return t('values.days', { count: n });
    }
    case 'date':
      return formatDate(fact.value);
    default:
      return fact.value;
  }
}

export function redactionPreview(sections: ComplianceSection[], selected: readonly string[]): ComplianceFact[] {
  const wanted = new Set(selected);
  return sections.filter((s) => wanted.has(s.id)).flatMap((s) => s.facts.filter((f) => f.sensitive && f.value !== ''));
}

export type ComplianceDocSection = {
  title: string;
  status: ComplianceStatus;
  statusLine: string;
  description: string;
  factHeader: [string, string];
  rows: [string, string][];
  findingsTitle: string;
  findings: string[];
};

export type ComplianceDocument = {
  title: string;
  meta: string[];
  sections: ComplianceDocSection[];
};

export function buildComplianceDocument(
  report: ComplianceReport,
  t: ComplianceT,
  formatDate: (iso: string) => string,
): ComplianceDocument {
  return {
    title: t('report.htmlTitle'),
    meta: [
      t('report.htmlVersion', { version: report.version, build: report.build }),
      t('report.htmlGenerated', { time: formatDate(report.generatedAt) }),
      report.redacted ? t('report.htmlRedacted') : t('report.htmlNotRedacted'),
    ],
    sections: report.sections.map((section) => ({
      title: t(`sections.${section.id}.title`),
      status: section.status,
      statusLine: `${t('report.htmlStatus')}: ${t(`status.${section.status}`)}`,
      description: t(`sections.${section.id}.description`),
      factHeader: [t('report.htmlFact'), t('report.htmlValue')],
      rows: section.facts.map((f): [string, string] => [t(`facts.${f.key}`), formatFactValue(f, t, formatDate)]),
      findingsTitle: t('report.htmlFindings'),
      findings: section.reasons.map((r) => t(`reasons.${r}`)),
    })),
  };
}

export type ComplianceExportFormat = 'json' | 'docx' | 'pdf';

export function reportFileName(report: ComplianceReport, format: ComplianceExportFormat): string {
  const stamp = report.generatedAt.replace(/[:]/g, '').replace(/\..*$/, '');
  return `mcharbor-compliance-${stamp}.${format}`;
}

export function downloadBlob(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  link.click();
  URL.revokeObjectURL(url);
}
