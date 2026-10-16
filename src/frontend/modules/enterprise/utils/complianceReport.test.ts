// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeAll, describe, expect, it } from 'vitest';
import { i18n, initTestI18n } from '../../../test/i18n';
import type { ComplianceReport, ComplianceSection } from '../hooks/useCompliance';
import JSZip from 'jszip';
import { pdfSafe, renderComplianceDocx, renderCompliancePdf } from './complianceExport';
import {
  buildComplianceDocument,
  formatFactValue,
  redactionPreview,
  reportFileName,
  type ComplianceT,
} from './complianceReport';

let t: ComplianceT;
beforeAll(async () => {
  await initTestI18n();
  t = i18n.getFixedT('en', 'compliance') as ComplianceT;
});

const iso = (v: string) => `D(${v})`;

const sections: ComplianceSection[] = [
  {
    id: 'keys',
    status: 'warn',
    facts: [
      { key: 'backupKeyConfigured', kind: 'bool', value: 'true', sensitive: false },
      { key: 'backupKeyId', kind: 'code', value: 'kid-1', sensitive: true },
    ],
    reasons: ['noBackupKey'],
  },
  {
    id: 'tls',
    status: 'pass',
    facts: [
      { key: 'subject', kind: 'text', value: '<script>alert(1)</script>', sensitive: true },
      { key: 'daysRemaining', kind: 'days', value: '0', sensitive: false },
    ],
    reasons: [],
  },
  { id: 'ha', status: 'info', facts: [{ key: 'nodeId', kind: 'code', value: '', sensitive: true }], reasons: [] },
];

const report: ComplianceReport = {
  generatedAt: '2026-09-26T12:00:00Z',
  version: '2026.09.01',
  build: '202609261200',
  redacted: false,
  summary: { pass: 1, warn: 1, fail: 0, info: 1, unknown: 0 },
  sections,
};

describe('compliance report formatting', () => {
  it('formats by kind, and a zero retention is no limit but zero days remaining is not', () => {
    expect(formatFactValue({ key: 'x', kind: 'bool', value: 'false', sensitive: false }, t, iso)).toBe('No');
    expect(formatFactValue({ key: 'mfaAdoption', kind: 'percent', value: '66', sensitive: false }, t, iso)).toBe('66%');
    expect(formatFactValue({ key: 'retentionDays', kind: 'days', value: '0', sensitive: false }, t, iso)).toBe(
      'No limit',
    );
    expect(formatFactValue({ key: 'daysRemaining', kind: 'days', value: '0', sensitive: false }, t, iso)).toBe(
      '0 days',
    );
    expect(formatFactValue({ key: 'retentionDays', kind: 'days', value: '1', sensitive: false }, t, iso)).toBe('1 day');
    expect(
      formatFactValue({ key: 'notAfter', kind: 'date', value: '2026-01-01T00:00:00Z', sensitive: false }, t, iso),
    ).toBe('D(2026-01-01T00:00:00Z)');
    expect(formatFactValue({ key: 'backupKeyId', kind: 'code', value: '[redacted]', sensitive: true }, t, iso)).toBe(
      '[redacted]',
    );
  });

  it('previews only non-empty sensitive facts in the selected sections', () => {
    expect(redactionPreview(sections, ['keys', 'tls', 'ha']).map((f) => f.key)).toEqual(['backupKeyId', 'subject']);
    expect(redactionPreview(sections, ['ha'])).toEqual([]);
    expect(redactionPreview(sections, ['keys']).map((f) => f.key)).toEqual(['backupKeyId']);
  });

  it('builds one document model with every section, fact, finding and the redaction notice', () => {
    const model = buildComplianceDocument(report, t, iso);
    expect(model.title).toBe('McHarbor compliance report');
    expect(model.meta).toContain('This report is not redacted.');
    expect(model.sections.map((s) => s.title)).toContain('Encryption keys');
    const [keys, tls] = model.sections;
    if (!keys || !tls) throw new Error('sections missing');
    expect(keys.status).toBe('warn');
    expect(keys.rows).toEqual([
      [t('facts.backupKeyConfigured'), 'Yes'],
      [t('facts.backupKeyId'), 'kid-1'],
    ]);
    expect(keys.findings).toEqual(['No backup encryption key is configured.']);
    expect(tls.rows[0]?.[1]).toBe('<script>alert(1)</script>');
    expect(buildComplianceDocument({ ...report, redacted: true }, t, iso).meta).toContain(
      'Identifying values are redacted.',
    );
  });

  it('writes a .docx whose document part carries the facts, values and findings', async () => {
    const blob = await renderComplianceDocx(buildComplianceDocument(report, t, iso));
    const zip = await JSZip.loadAsync(await blob.arrayBuffer());
    const xml = await zip.file('word/document.xml')?.async('string');
    expect(xml).toBeDefined();
    expect(xml).toContain('Encryption keys');
    expect(xml).toContain('kid-1');
    expect(xml).toContain('No backup encryption key is configured.');
    expect(xml).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
  });

  it('writes a .pdf carrying the sections and findings, with text outside the PDF font mapped rather than garbled', async () => {
    const blob = await renderCompliancePdf(buildComplianceDocument(report, t, iso));
    const text = new TextDecoder('latin1').decode(await blob.arrayBuffer());
    expect(text.startsWith('%PDF-')).toBe(true);
    expect(text).toContain('Encryption keys');
    expect(text).toContain('No backup encryption key is configured.');
    expect(pdfSafe('Settings → System ≥ 2 ✦ é €')).toBe('Settings -> System >= 2 ? é €');
  });

  it('names the file after the report time without characters Windows rejects', () => {
    expect(reportFileName(report, 'json')).toBe('mcharbor-compliance-2026-09-26T120000Z.json');
    expect(reportFileName(report, 'docx')).toBe('mcharbor-compliance-2026-09-26T120000Z.docx');
    expect(reportFileName(report, 'pdf')).not.toMatch(/[:]/);
  });
});
