// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { useSubjects } from '../hooks/useReporting';
import type { SubjectKind } from '../types';
import { subjectAllLabel, subjectKindLabel } from './catalog-labels';

// The scope picker.
//
// Several reports answer a question about one object — this workload,
// this user, this group — and the estate-wide version of that report is a
// poor way to read a single row. Where the catalog says a type can be
// narrowed, this offers the objects the signed-in user can already see.
//
// The default is deliberately "everything": scoping is the exception, and
// a picker that started on the first container would quietly change what
// every existing report means.

export type SubjectSelection = { kind: SubjectKind; id: string; name: string };

type Props = {
  /** The kinds the chosen report type offers, in catalog order. */
  kinds: SubjectKind[];
  /** The environment the objects are listed from. */
  envId: string;
  /**
   * The selection to open on. Null — the studio's case — starts at "the
   * whole estate". The edit dialog passes the scope its schedule was saved
   * with, because a form that silently reopened on "everything" would
   * widen a narrowed report the first time it was saved.
   */
  initial?: SubjectSelection | null;
  /**
   * The i18n namespace the kind labels resolve against. Set only for a
   * report type another plugin contributed: its scope kinds are its own
   * objects — a cluster, a namespace — and this plugin's bundle has no
   * word for them.
   */
  labelNs?: string;
  onChange: (subject: SubjectSelection | null) => void;
};

const ALL = '';

export function SubjectField({ kinds, envId, initial = null, labelNs, onChange }: Props) {
  const { t } = useTranslation('reporting');
  const [kind, setKind] = useState<SubjectKind | null>(initial?.kind ?? kinds[0] ?? null);
  const [id, setId] = useState<string>(initial?.id ?? ALL);

  const subjects = useSubjects(kind, envId);
  const items = useMemo(() => subjects.data?.items ?? [], [subjects.data]);

  // Switching kind invalidates the chosen object, and the parent has to
  // hear about that or it would keep submitting a container id under a
  // "user" scope.
  //
  // The first run is skipped. It fires on mount, where there is nothing to
  // invalidate — and where clearing would throw away a seeded `initial`
  // before the operator has touched anything. The studio is unaffected: it
  // opens with no selection, so the mount-time `onChange(null)` this used
  // to emit was telling the parent something it already knew.
  const settled = useRef(false);
  useEffect(() => {
    if (!settled.current) {
      settled.current = true;
      return;
    }
    setId(ALL);
    onChange(null);
  }, [kind, envId, onChange]);

  const options = useMemo(() => {
    const rows = items.map((s) => ({
      value: s.id,
      label: s.hint ? `${s.name} · ${s.hint}` : s.name,
    }));
    // A seeded object that no longer appears in the listing — deleted, or
    // moved to another environment — still has to be selectable, or the
    // Select would silently fall back to "everything" and the next save
    // would widen the report without anyone choosing to.
    if (initial && initial.id && kind === initial.kind && !items.some((s) => s.id === initial.id)) {
      rows.unshift({ value: initial.id, label: initial.name || initial.id });
    }
    return [{ value: ALL, label: kind ? subjectAllLabel(t, kind, labelNs) : '' }, ...rows];
  }, [initial, items, kind, labelNs, t]);

  // The parent only renders this where the catalog declares a scope, so
  // an empty list is a stale catalog rather than a state worth drawing.
  if (!kind) return null;

  const select = (next: string) => {
    setId(next);
    if (next === ALL) {
      onChange(null);
      return;
    }
    const chosen = items.find((s) => s.id === next);
    if (chosen) {
      onChange({ kind, id: chosen.id, name: chosen.name });
      return;
    }
    // The seeded-but-missing row above. Keep the name it was saved with
    // rather than reporting a nameless id.
    onChange(initial && initial.id === next ? { ...initial, kind } : null);
  };

  return (
    <>
      {kinds.length > 1 ? (
        <div>
          <p className="mb-1.5 text-xs font-semibold text-muted-foreground">{t('studio.subjectKind')}</p>
          <Select
            value={kind}
            onChange={(next) => setKind(next as SubjectKind)}
            ariaLabel={t('studio.subjectKind')}
            searchable={false}
            options={kinds.map((k) => ({ value: k, label: subjectKindLabel(t, k, labelNs) }))}
          />
        </div>
      ) : null}

      <div>
        <p className="mb-1.5 text-xs font-semibold text-muted-foreground">{t('studio.subject')}</p>
        <Select
          value={id}
          onChange={select}
          ariaLabel={t('studio.subject')}
          disabled={subjects.isLoading}
          options={options}
        />
        <p className="mt-1.5 text-xs text-muted-foreground">
          {!subjects.isLoading && items.length === 0 ? t('studio.subjectEmpty') : t('studio.subjectHint')}
        </p>
      </div>
    </>
  );
}
