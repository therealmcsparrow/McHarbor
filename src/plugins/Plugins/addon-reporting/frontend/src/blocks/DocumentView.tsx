// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { Block, ReportDocument, Section } from '../types';
import { ChartBlock } from './ChartBlock';
import { DataTable } from './DataTableBlock';
import { KpiRow, Callout, Findings, KeyValues, ScorecardBlock } from './Blocks';

// The on-screen report.
//
// The same document the PDF and Word exporters consume, rendered with
// the host's theme tokens so it belongs in the app rather than looking
// like a preview of a file. The page-break and pagination blocks the
// print renderers honour are ignored here — a screen has no pages.

type Props = { doc: ReportDocument };

/**
 * DOM id of a section. Keyed by index as well as level because a report may
 * carry several sections at the same altitude, and two elements sharing an
 * id break both the rail and in-page anchors.
 */
export function sectionAnchorId(level: string, index: number): string {
  return `section-${level}-${index}`;
}

export function DocumentView({ doc }: Props) {
  return (
    <article className="mx-auto w-full max-w-4xl">
      {doc.sections.map((section, i) => (
        <SectionView key={`${section.level}-${i}`} doc={doc} section={section} index={i} />
      ))}
    </article>
  );
}

function SectionView({ doc, section, index }: { doc: ReportDocument; section: Section; index: number }) {
  const { t } = useTranslation('reporting');
  return (
    <section
      id={sectionAnchorId(section.level, index)}
      className="scroll-mt-24 border-t border-border pt-8 first:border-t-0 first:pt-0"
    >
      <p className="text-[11px] font-semibold tracking-[0.14em] text-primary uppercase">
        {t(`level.${section.level}`)}
      </p>
      <h2 className="mt-1.5 text-2xl font-bold text-foreground">{section.title}</h2>
      {section.intent ? <p className="mt-1 text-sm text-muted-foreground">{section.intent}</p> : null}
      <div className="mt-5 pb-8">
        {section.blocks.map((block, position) => (
          // A rendered document is immutable, so a block's position is its identity.
          <BlockView key={position} doc={doc} block={block} />
        ))}
      </div>
    </section>
  );
}

function BlockView({ doc, block }: { doc: ReportDocument; block: Block }) {
  switch (block.kind) {
    case 'heading':
      return <HeadingBlock depth={block.depth ?? 2} text={block.text ?? ''} />;

    case 'paragraph':
      return (
        <p
          className={
            block.lead
              ? 'mb-4 text-base leading-relaxed text-foreground'
              : 'mb-3 text-sm leading-relaxed text-foreground'
          }
        >
          {block.text}
        </p>
      );

    case 'list':
      return block.ordered ? (
        <ol className="mb-4 ml-1 list-inside list-decimal space-y-1.5 text-sm leading-relaxed text-foreground marker:font-semibold marker:text-primary">
          {(block.items ?? []).map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ol>
      ) : (
        <ul className="mb-4 space-y-1.5 text-sm leading-relaxed text-foreground">
          {(block.items ?? []).map((item) => (
            <li key={item} className="flex gap-2.5">
              <span aria-hidden="true" className="mt-[0.45rem] size-1.5 shrink-0 rounded-full bg-primary" />
              <span>{item}</span>
            </li>
          ))}
        </ul>
      );

    case 'kpis':
      return <KpiRow items={block.kpis ?? []} />;

    case 'table':
      return (
        <DataTable columns={block.columns ?? []} rows={block.rows ?? []} caption={block.caption} note={block.note} />
      );

    case 'chart':
      return block.chart ? <ChartBlock chart={block.chart} /> : null;

    case 'callout':
      return <Callout tone={block.tone ?? ''} title={block.title} text={block.text ?? ''} />;

    case 'keyvalue':
      return <KeyValues title={block.title} pairs={block.pairs ?? []} />;

    case 'findings':
      return <Findings items={block.findings ?? []} />;

    case 'scorecard':
      return doc.scorecard ? <ScorecardBlock scorecard={doc.scorecard} /> : null;

    case 'divider':
      return <hr className="my-6 border-border" />;

    // A screen has no pages, so the print-only blocks render nothing.
    case 'pagebreak':
      return null;

    default:
      return null;
  }
}

function HeadingBlock({ depth, text }: { depth: number; text: string }) {
  if (!text) return null;
  if (depth <= 1) {
    return <h3 className="mt-7 mb-3 text-xl font-bold text-foreground">{text}</h3>;
  }
  if (depth === 2) {
    return <h4 className="mt-6 mb-2.5 text-base font-bold text-foreground">{text}</h4>;
  }
  return <h5 className="mt-5 mb-2 text-sm font-semibold text-primary">{text}</h5>;
}
