// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBoxMultiple } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import type { CreatePolicyInput } from '../hooks/useUpdates';
import { ALL_CONTAINERS, PolicyContainersDialog, parseContainerMatch } from './PolicyContainersDialog';

const SUMMARY_LIMIT = 6;

const EMPTY_FORM: CreatePolicyInput = {
  name: '',
  containerMatch: ALL_CONTAINERS,
  imageMatch: '*',
  schedule: '0 3 * * *',
  strategy: 'latest',
  autoRestart: true,
};

type Props = {
  creating: boolean;
  onCreate: (input: CreatePolicyInput) => Promise<unknown>;
  onCancel: () => void;
};

// CreateUpdatePolicyForm is the inline "new policy" panel. Container
// targeting is delegated to PolicyContainersDialog; the form only shows a
// summary of the picked names and a button to open the picker.
export function CreateUpdatePolicyForm({ creating, onCreate, onCancel }: Props) {
  const { t } = useTranslation('settings');
  const [form, setForm] = useState<CreatePolicyInput>(EMPTY_FORM);
  const [pickerOpen, setPickerOpen] = useState(false);

  const names = parseContainerMatch(form.containerMatch);
  const visible = names.slice(0, SUMMARY_LIMIT);
  const hidden = names.length - visible.length;

  const strategyOptions = [
    { value: 'latest', label: t('updates.strategyLatest') },
    { value: 'semver', label: t('updates.strategySemver') },
    { value: 'digest', label: t('updates.strategyDigest') },
  ];

  const handleCreate = () => {
    if (!form.name.trim()) return;
    void onCreate(form).then(() => setForm(EMPTY_FORM));
  };

  return (
    <div className="mc-glass space-y-4 rounded-lg p-6">
      <h4 className="text-sm font-semibold text-foreground">{t('updates.createTitle')}</h4>
      <p className="text-sm text-muted-foreground">{t('updates.createDescription')}</p>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <Label className="mb-1.5">{t('updates.nameLabel')}</Label>
          <Input
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder={t('updates.namePlaceholder')}
          />
        </div>
        <ReadableScheduleField
          value={form.schedule}
          onChange={(schedule) => setForm({ ...form, schedule })}
          label={t('updates.scheduleLabel')}
          hint={t('updates.scheduleHint')}
        />
      </div>

      <div>
        <Label className="mb-1.5">{t('updates.containerMatchLabel')}</Label>
        <div className="mt-1.5 flex min-h-[42px] flex-wrap items-center gap-1.5 rounded-lg border border-border bg-muted/50 p-2">
          {names.length === 0 ? (
            <Badge variant="default" className="px-2 py-0.5 text-xs">
              {t('updates.allContainers')}
            </Badge>
          ) : (
            <>
              {visible.map((policyName) => (
                <Badge key={policyName} variant="outline" className="px-2 py-0.5 text-xs">
                  {policyName}
                </Badge>
              ))}
              {hidden > 0 && (
                <Badge variant="secondary" className="px-2 py-0.5 text-xs">
                  {t('updates.moreContainers', { count: hidden })}
                </Badge>
              )}
            </>
          )}
          <Button variant="outline" size="sm" className="ml-auto" onClick={() => setPickerOpen(true)}>
            <IconBoxMultiple className="size-4" />
            {t('updates.selectContainers')}
          </Button>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{t('updates.containerSelectHint')}</p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <Label className="mb-1.5">{t('updates.imageMatchLabel')}</Label>
          <Input
            value={form.imageMatch}
            onChange={(e) => setForm({ ...form, imageMatch: e.target.value })}
            placeholder="*"
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('updates.imageMatchHint')}</p>
        </div>
        <div>
          <Label className="mb-1.5">{t('updates.strategyLabel')}</Label>
          <Select
            value={form.strategy}
            onChange={(v) => setForm({ ...form, strategy: v })}
            options={strategyOptions}
            className="w-full"
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('updates.strategyHint')}</p>
        </div>
        <div className="flex items-center gap-3 pt-2">
          <Switch checked={form.autoRestart} onCheckedChange={(v) => setForm({ ...form, autoRestart: v })} />
          <Label>{t('updates.autoRestartLabel')}</Label>
        </div>
      </div>

      <div className="flex gap-2 pt-2">
        <Button permission={PERM.UPDATES_MANAGE} onClick={handleCreate} disabled={creating || !form.name.trim()}>
          {creating ? t('updates.creating') : t('actions.create', { ns: 'common' })}
        </Button>
        <Button variant="outline" onClick={onCancel}>
          {t('actions.cancel', { ns: 'common' })}
        </Button>
      </div>

      <PolicyContainersDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        value={form.containerMatch}
        onSave={(containerMatch) => {
          setForm((prev) => ({ ...prev, containerMatch }));
          setPickerOpen(false);
        }}
      />
    </div>
  );
}
