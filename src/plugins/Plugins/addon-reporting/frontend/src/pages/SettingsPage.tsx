// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { useCatalog, useSaveSettings, useSettings } from '../hooks/useReporting';
import { MarkField } from './MarkField';
import type { PageSizeKey, PageSizeOption, Settings } from '../types';

// Reporting settings.
//
// These values are what appears on the cover of a document that leaves the
// building, plus how much history to keep. Small surface on purpose: the
// report's substance is derived from the platform, not configured here.

// A cover wordmark prints a couple of inches wide and the header mark a few
// millimetres; these are the longest edge each is rasterised at, which is
// already past what a reader could resolve on paper.
const wordmarkSide = 1600;
const markSide = 512;

// The papers offered when the catalog has not answered yet — the same two
// the backend knows, so the field is usable on a cold load rather than
// empty.
const fallbackPaper: PageSizeOption[] = [
  { key: 'a4', label: 'A4', width: 595.276, height: 841.89 },
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: paper size name, as printed on the stock
  { key: 'legal', label: 'Legal', width: 612, height: 1008 },
];

export function SettingsPage() {
  const { t } = useTranslation('reporting');
  const settings = useSettings();
  const catalog = useCatalog();
  const save = useSaveSettings();
  const [draft, setDraft] = useState<Settings | null>(null);

  if (settings.data && draft === null) setDraft(settings.data);

  if (settings.isLoading || draft === null) return <CardSkeleton lines={6} />;

  const update = <K extends keyof Settings>(key: K, value: Settings[K]) => {
    setDraft((prev) => (prev ? { ...prev, [key]: value } : prev));
  };

  // The label carries the dimensions as well as the name: "Legal" means
  // nothing to a reader outside North America, and "A4" means nothing to
  // one inside it.
  const papers = catalog.data?.page_sizes ?? fallbackPaper;
  const paperOptions = papers.map((paper) => ({
    value: paper.key,
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: paper sizes come from the backend catalog and carry their own label
    label: t(`option.paper.${paper.key}`, { defaultValue: paper.label }),
  }));

  return (
    <div className="max-w-2xl space-y-6">
      <Field
        label={t('settings.organisation.label')}
        help={t('settings.organisation.help')}
        control={
          <Input
            value={draft.organisation}
            maxLength={80}
            onChange={(e) => update('organisation', e.target.value)}
            aria-label={t('settings.organisation.label')}
          />
        }
      />

      <Field
        label={t('settings.classification.label')}
        help={t('settings.classification.help')}
        control={
          <Input
            value={draft.classification}
            maxLength={40}
            onChange={(e) => update('classification', e.target.value)}
            aria-label={t('settings.classification.label')}
          />
        }
      />

      <MarkField
        label={t('settings.logo.label')}
        help={t('settings.logo.help')}
        value={draft.logo_png}
        defaultSrc="/logo_McHarbor.svg"
        maxSide={wordmarkSide}
        wide
        onChange={(next) => update('logo_png', next)}
      />

      <MarkField
        label={t('settings.icon.label')}
        help={t('settings.icon.help')}
        value={draft.icon_png}
        defaultSrc="/icon_McSparrow.svg"
        maxSide={markSide}
        onChange={(next) => update('icon_png', next)}
      />

      <Field
        label={t('settings.paper.label')}
        help={t('settings.paper.help')}
        control={
          <Select
            value={draft.page_size}
            onChange={(value) => update('page_size', value as PageSizeKey)}
            options={paperOptions}
            ariaLabel={t('settings.paper.label')}
          />
        }
      />

      <Field
        label={t('settings.defaultDays.label')}
        help={t('settings.defaultDays.help')}
        control={
          <NumberInput
            value={draft.default_days}
            min={1}
            max={365}
            onChange={(v) => update('default_days', v ?? 30)}
            aria-label={t('settings.defaultDays.label')}
          />
        }
      />

      <Field
        label={t('settings.retain.label')}
        help={t('settings.retain.help')}
        control={
          <NumberInput
            value={draft.retain_reports}
            min={10}
            max={1000}
            onChange={(v) => update('retain_reports', v ?? 100)}
            aria-label={t('settings.retain.label')}
          />
        }
      />

      <Field
        label={t('settings.ai.label')}
        help={t('settings.ai.help')}
        control={
          <Switch
            checked={draft.ai_narrative}
            onCheckedChange={(v) => update('ai_narrative', v)}
            aria-label={t('settings.ai.label')}
          />
        }
      />

      <Field
        label={t('settings.aiReports.label')}
        help={t('settings.aiReports.help')}
        control={
          <Switch
            checked={draft.ai_reports}
            onCheckedChange={(v) => update('ai_reports', v)}
            aria-label={t('settings.aiReports.label')}
          />
        }
      />

      <div className="flex items-center gap-3 border-t border-border pt-5">
        <Button permission="reporting.manage" onClick={() => save.mutate(draft)} disabled={save.isPending}>
          {save.isPending ? t('settings.saving') : t('action.save')}
        </Button>
        {save.isSuccess ? <span className="text-sm text-success">{t('settings.saved')}</span> : null}
        {save.isError ? <span className="text-sm text-danger">{t('settings.error')}</span> : null}
      </div>
    </div>
  );
}

function Field({ label, help, control }: { label: string; help: string; control: React.ReactNode }) {
  return (
    <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_16rem] sm:items-start sm:gap-6">
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{help}</p>
      </div>
      <div>{control}</div>
    </div>
  );
}
