// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Disclosure } from '@resources/components/ui/Disclosure';
import type { Evidence, EvidenceSection } from '@resources/hooks/useIncidents';

const KNOWN_NOTES = ['plugin_unloaded', 'plugin_unavailable'];

function SectionBody({ section }: { section: EvidenceSection }) {
  const { t } = useTranslation('incidents');
  const note = section.note
    ? KNOWN_NOTES.includes(section.note)
      ? t(`evidence.pluginNote.${section.note}`)
      : section.note
    : '';
  return (
    <div className="space-y-2">
      {note && <p className="text-xs text-muted-foreground">{note}</p>}
      {section.kind === 'log' && (section.lines?.length ?? 0) > 0 && (
        <pre className="max-h-96 overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap">
          {section.lines?.join('\n')}
        </pre>
      )}
      {section.kind === 'text' &&
        section.lines?.map((line, i) => (
          <p key={`${section.title}-${String(i)}`} className="text-sm">
            {line}
          </p>
        ))}
      {section.kind === 'table' && (section.rows?.length ?? 0) > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            {section.columns && (
              <thead>
                <tr className="border-b border-border text-xs text-muted-foreground">
                  {section.columns.map((c) => (
                    <th key={c} className="py-1 pr-4 font-medium">
                      {c}
                    </th>
                  ))}
                </tr>
              </thead>
            )}
            <tbody className="divide-y divide-border">
              {section.rows?.map((row, r) => (
                <tr key={`${section.title}-${String(r)}`}>
                  {row.map((cell, c) => (
                    <td key={`${String(r)}-${String(c)}`} className="py-1 pr-4 break-all">
                      {cell}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function sectionSize(s: EvidenceSection) {
  return (s.lines?.length ?? 0) + (s.rows?.length ?? 0);
}

export function PluginEvidence({ evidence }: { evidence: Evidence }) {
  const { t } = useTranslation('incidents');
  const facts = Object.entries(evidence.subject?.facts ?? {});
  return (
    <>
      {facts.length > 0 && (
        <dl className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm md:grid-cols-3">
          {facts.map(([label, value]) => (
            <div key={label} className="flex gap-2">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="font-mono break-all">{value || '—'}</dd>
            </div>
          ))}
        </dl>
      )}
      {(evidence.sections ?? []).map((section) => (
        <Disclosure
          key={section.title}
          title={section.title}
          trailing={<Badge variant="secondary">{sectionSize(section)}</Badge>}
          defaultOpen={sectionSize(section) > 0 || Boolean(section.note)}
        >
          {sectionSize(section) === 0 && !section.note ? (
            <p className="text-sm text-muted-foreground">{t('evidence.nothing')}</p>
          ) : (
            <SectionBody section={section} />
          )}
        </Disclosure>
      ))}
    </>
  );
}
