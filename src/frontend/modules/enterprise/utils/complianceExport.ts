// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComplianceStatus } from '../hooks/useCompliance';
import type { ComplianceDocument } from './complianceReport';

type Rgb = [number, number, number];

const STATUS_RGB: Partial<Record<ComplianceStatus, Rgb>> = {
  fail: [178, 34, 34],
  warn: [184, 134, 11],
  pass: [0, 100, 0],
};

const STATUS_HEX: Partial<Record<ComplianceStatus, string>> = {
  fail: 'B22222',
  warn: 'B8860B',
  pass: '006400',
};

export async function renderComplianceDocx(modelName: ComplianceDocument): Promise<Blob> {
  const {
    AlignmentType,
    BorderStyle,
    Document: DocumentItem,
    HeadingLevel,
    Packer,
    Paragraph,
    Table,
    TableCell,
    TableRow,
    TextRun,
    WidthType,
  } = await import('docx');

  const cellBorder = { style: BorderStyle.SINGLE, size: 4, color: 'D9D9D9' };
  const borders = { top: cellBorder, bottom: cellBorder, left: cellBorder, right: cellBorder };
  const cell = (text: string, bold: boolean, width: number) =>
    new TableCell({
      borders,
      width: { size: width, type: WidthType.PERCENTAGE },
      children: [new Paragraph({ children: [new TextRun({ text, bold })] })],
    });

  const children: (InstanceType<typeof Paragraph> | InstanceType<typeof Table>)[] = [
    new Paragraph({ heading: HeadingLevel.TITLE, children: [new TextRun(modelName.title)] }),
    ...modelName.meta.map((line) => new Paragraph({ children: [new TextRun({ text: line, color: '595959' })] })),
  ];

  for (const section of modelName.sections) {
    const color = STATUS_HEX[section.status];
    children.push(
      new Paragraph({
        heading: HeadingLevel.HEADING_1,
        spacing: { before: 320 },
        children: [new TextRun(section.title)],
      }),
      new Paragraph({ children: [new TextRun({ text: section.statusLine, bold: true, color })] }),
      new Paragraph({ spacing: { after: 120 }, children: [new TextRun(section.description)] }),
      new Table({
        width: { size: 100, type: WidthType.PERCENTAGE },
        rows: [
          new TableRow({
            tableHeader: true,
            children: [cell(section.factHeader[0], true, 40), cell(section.factHeader[1], true, 60)],
          }),
          ...section.rows.map(
            ([fact, value]) => new TableRow({ children: [cell(fact, true, 40), cell(value, false, 60)] }),
          ),
        ],
      }),
    );
    if (section.findings.length > 0) {
      children.push(
        new Paragraph({
          heading: HeadingLevel.HEADING_2,
          spacing: { before: 200 },
          children: [new TextRun(section.findingsTitle)],
        }),
        ...section.findings.map(
          (f) => new Paragraph({ bullet: { level: 0 }, alignment: AlignmentType.LEFT, children: [new TextRun(f)] }),
        ),
      );
    }
  }

  const doc = new DocumentItem({
    title: modelName.title,
    creator: 'McHarbor',
    styles: { default: { document: { run: { font: 'Calibri', size: 21 } } } },
    sections: [{ children }],
  });
  return Packer.toBlob(doc);
}

const CP1252_EXTRA = '€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ';
const PDF_REPLACEMENTS: Record<string, string> = {
  '→': '->',
  '←': '<-',
  '≥': '>=',
  '≤': '<=',
  '≠': '!=',
  '✓': 'v',
  '×': 'x',
};

export function pdfSafe(text: string): string {
  let out = '';
  for (const ch of text) {
    const code = ch.codePointAt(0) ?? 0;
    if (code < 0x100 || CP1252_EXTRA.includes(ch)) out += ch;
    else out += PDF_REPLACEMENTS[ch] ?? '?';
  }
  return out;
}

export async function renderCompliancePdf(modelValue: ComplianceDocument): Promise<Blob> {
  const { jsPDF } = await import('jspdf');
  const doc = new jsPDF({ unit: 'pt', format: 'a4' });
  const margin = 48;
  const pageWidth = doc.internal.pageSize.getWidth();
  const pageHeight = doc.internal.pageSize.getHeight();
  const width = pageWidth - margin * 2;
  const bottom = pageHeight - margin;
  let y = margin;

  const ensure = (height: number) => {
    if (y + height > bottom) {
      doc.addPage();
      y = margin;
    }
  };
  const write = (text: string, size: number, style: 'normal' | 'bold', rgb: Rgb = [0, 0, 0], gap = 4) => {
    doc.setFont('helvetica', style);
    doc.setFontSize(size);
    doc.setTextColor(rgb[0], rgb[1], rgb[2]);
    const lines = doc.splitTextToSize(pdfSafe(text), width) as string[];
    const lead = size * 1.3;
    for (const line of lines) {
      ensure(lead);
      doc.text(line, margin, y + size);
      y += lead;
    }
    y += gap;
  };

  const colA = width * 0.4;
  const colB = width - colA;
  const pad = 4;
  const tableRow = (a: string, b: string, head: boolean) => {
    doc.setFontSize(9);
    doc.setFont('helvetica', 'bold');
    const linesA = doc.splitTextToSize(pdfSafe(a), colA - pad * 2) as string[];
    doc.setFont('helvetica', head ? 'bold' : 'normal');
    const linesB = doc.splitTextToSize(pdfSafe(b), colB - pad * 2) as string[];
    const lead = 9 * 1.3;
    const height = Math.max(linesA.length, linesB.length) * lead + pad * 2;
    ensure(height);
    if (head) {
      doc.setFillColor(242, 242, 242);
      doc.rect(margin, y, width, height, 'F');
    }
    doc.setTextColor(0, 0, 0);
    doc.setFont('helvetica', 'bold');
    doc.text(linesA, margin + pad, y + pad + 9);
    doc.setFont('helvetica', head ? 'bold' : 'normal');
    doc.text(linesB, margin + colA + pad, y + pad + 9);
    doc.setDrawColor(217, 217, 217);
    doc.line(margin, y + height, margin + width, y + height);
    y += height;
  };

  write(modelValue.title, 20, 'bold', [0, 0, 0], 6);
  for (const line of modelValue.meta) write(line, 10, 'normal', [89, 89, 89], 0);
  y += 8;

  for (const section of modelValue.sections) {
    ensure(80);
    doc.setDrawColor(191, 191, 191);
    doc.line(margin, y, margin + width, y);
    y += 10;
    write(section.title, 14, 'bold', [0, 0, 0], 2);
    write(section.statusLine, 10, 'bold', STATUS_RGB[section.status] ?? [0, 0, 0], 2);
    write(section.description, 10, 'normal', [0, 0, 0], 6);
    tableRow(section.factHeader[0], section.factHeader[1], true);
    for (const [fact, value] of section.rows) tableRow(fact, value, false);
    y += 8;
    if (section.findings.length > 0) {
      write(section.findingsTitle, 11, 'bold', [0, 0, 0], 2);
      for (const f of section.findings) write(`• ${f}`, 10, 'normal', [0, 0, 0], 2);
      y += 6;
    }
  }

  return doc.output('blob');
}
