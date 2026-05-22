// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@resources/components/ui/Card';
import { Label } from '@resources/components/ui/Label';
import { CronSchedulePreview } from '@resources/components/CronSchedulePreview';
import { Select, type SelectOption } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import { SUPPORTED_TIMEZONES } from '../timezones';

function buildTimezoneOptions(): SelectOption[] {
  // Keep the browser out of timezone discovery so frontend choices stay aligned with Go validation.
  return SUPPORTED_TIMEZONES.map((timezone) => ({ value: timezone, label: timezone }));
}

const TIMEZONE_OPTIONS = buildTimezoneOptions();

type ToggleSettingRowProps = {
  title: string;
  description: string;
  hint: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
  statusOn: string;
  statusOff: string;
};

function ToggleSettingRow({
  title,
  description,
  hint,
  checked,
  disabled = false,
  onCheckedChange,
  statusOn,
  statusOff,
}: ToggleSettingRowProps) {
  return (
    <div className="flex flex-col gap-4 rounded-xl border border-border bg-muted/20 p-4 md:flex-row md:items-center md:justify-between">
      <div className="space-y-1">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            {checked ? statusOn : statusOff}
          </span>
        </div>
        <p className="text-sm text-muted-foreground">{description}</p>
        <p className="text-xs uppercase tracking-[0.14em] text-muted-foreground/80">{hint}</p>
      </div>

      <Switch checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
    </div>
  );
}

type EnvironmentAutomationTabProps = {
  env: EnvironmentInfo;
  scheduledUpdateCheckEnabled: boolean;
  automaticImagePruningEnabled: boolean;
  timezone: string;
  isSaving: boolean;
  onScheduledUpdateCheckChange: (checked: boolean) => void;
  onAutomaticImagePruningChange: (checked: boolean) => void;
  onTimezoneChange: (value: string) => void;
};

export function EnvironmentAutomationTab({
  env,
  scheduledUpdateCheckEnabled,
  automaticImagePruningEnabled,
  timezone,
  isSaving,
  onScheduledUpdateCheckChange,
  onAutomaticImagePruningChange,
  onTimezoneChange,
}: EnvironmentAutomationTabProps) {
  const { t } = useTranslation('environments');

  const isDocker = env.orchestratorType === 'docker';

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.automation.title')}</CardTitle>
        <CardDescription>{t('detail.automation.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!isDocker && (
          <div className="rounded-xl border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
            {t('detail.automation.kubernetesNote')}
          </div>
        )}

        <ToggleSettingRow
          title={t('detail.automation.scheduledUpdateCheckTitle')}
          description={t('detail.automation.scheduledUpdateCheckDescription')}
          hint={t('detail.automation.scheduledUpdateCheckHint')}
          checked={scheduledUpdateCheckEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onScheduledUpdateCheckChange}
          statusOn={t('detail.automation.statusOn')}
          statusOff={t('detail.automation.statusOff')}
        />

        <ToggleSettingRow
          title={t('detail.automation.automaticImagePruningTitle')}
          description={t('detail.automation.automaticImagePruningDescription')}
          hint={t('detail.automation.automaticImagePruningHint')}
          checked={automaticImagePruningEnabled}
          disabled={!isDocker || isSaving}
          onCheckedChange={onAutomaticImagePruningChange}
          statusOn={t('detail.automation.statusOn')}
          statusOff={t('detail.automation.statusOff')}
        />

        <div className="rounded-xl border border-border bg-muted/20 p-4">
          <Label className="mb-2 block text-sm font-semibold text-foreground">
            {t('detail.automation.timezoneLabel')}
          </Label>
          <p className="mb-3 text-sm text-muted-foreground">{t('detail.automation.timezoneDescription')}</p>
          <Select
            value={timezone}
            onChange={onTimezoneChange}
            options={TIMEZONE_OPTIONS}
            className="max-w-md"
          />
          <div className="mt-4 space-y-2">
            <p className="text-xs uppercase tracking-[0.14em] text-muted-foreground/80">
              {t('detail.automation.prunePreviewLabel')}
            </p>
            <CronSchedulePreview expression="0 3 * * *" timezone={timezone} className="rounded-md border border-border/60 bg-card/60 p-2" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
