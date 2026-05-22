// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconPlus,
  IconTrash,
  IconPlayerPlay,
  IconPlayerPause,
  IconX,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { CronSchedulePreview } from '@resources/components/CronSchedulePreview';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useContainers } from '@resources/hooks/useContainers';
import { timeAgo } from '@resources/utils/format';
import {
  useUpdatePolicies,
  useCreateUpdatePolicy,
  useUpdateUpdatePolicy,
  useDeleteUpdatePolicy,
  type CreatePolicyInput,
} from '../hooks/useUpdates';

export function UpdatesTab() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: policies = [] } = useUpdatePolicies();
  const { data: containers = [] } = useContainers();
  const createPolicy = useCreateUpdatePolicy();
  const updatePolicy = useUpdateUpdatePolicy();
  const deletePolicy = useDeleteUpdatePolicy();
  const [showDialog, setShowDialog] = useState(false);
  const [selectedContainers, setSelectedContainers] = useState<string[]>([]);
  const [allContainers, setAllContainers] = useState(true);

  const containerNames = containers.map((c) => c.Names?.[0]?.replace(/^\//, '') ?? c.Id.slice(0, 12));

  const toggleContainer = (name: string) => {
    setSelectedContainers((prev) =>
      prev.includes(name) ? prev.filter((n) => n !== name) : [...prev, name]
    );
    setAllContainers(false);
  };

  const toggleAllContainers = () => {
    setAllContainers(true);
    setSelectedContainers([]);
  };

  const [form, setForm] = useState<Omit<CreatePolicyInput, 'containerMatch'>>({
    name: '',
    imageMatch: '*',
    schedule: '0 3 * * *',
    strategy: 'latest',
    autoRestart: true,
  });

  const handleCreate = () => {
    if (!form.name.trim()) return;
    const containerMatch = allContainers ? '*' : selectedContainers.join(',');
    createPolicy.mutate({ ...form, containerMatch }, {
      onSuccess: () => {
        setShowDialog(false);
        setForm({
          name: '',
          imageMatch: '*',
          schedule: '0 3 * * *',
          strategy: 'latest',
          autoRestart: true,
        });
        setSelectedContainers([]);
        setAllContainers(true);
      },
    });
  };

  const handleToggle = (id: string, enabled: boolean) => {
    updatePolicy.mutate({ id, enabled: !enabled });
  };

  const handleDelete = (id: string) => {
    deletePolicy.mutate(id);
  };

  const strategyOptions = [
    { value: 'latest', label: t('updates.strategyLatest') },
    { value: 'semver', label: t('updates.strategySemver') },
    { value: 'digest', label: t('updates.strategyDigest') },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('updates.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('updates.description')}</p>
        </div>
        <Button onClick={() => setShowDialog(true)}>
          <IconPlus className="size-4" />
          {t('updates.addPolicy')}
        </Button>
      </div>

      {policies.length === 0 && !showDialog && (
        <p className="py-8 text-center text-sm text-muted-foreground">{t('updates.noPolicies')}</p>
      )}

      {/* Policy list */}
      <div className="space-y-3">
        {policies.map((policy) => (
          <div key={policy.id} className="flex items-center justify-between rounded-lg border border-border bg-card p-4">
            <div className="flex-1 space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-foreground">{policy.name}</span>
                <Badge variant={policy.enabled ? 'default' : 'secondary'}>
                  {policy.enabled ? t('updates.enabled') : t('updates.disabled')}
                </Badge>
                <Badge variant="outline">{policy.strategy}</Badge>
                {policy.lastRunStatus && (
                  <Badge variant={policy.lastRunStatus === 'success' ? 'default' : 'destructive'}>
                    {policy.lastRunStatus}
                  </Badge>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span className="flex items-center gap-1">
                  {t('updates.containerPattern')}:
                  {(!policy.containerMatch || policy.containerMatch === '*') ? (
                    <Badge variant="outline" className="text-[10px] px-1.5 py-0">{t('updates.allContainers')}</Badge>
                  ) : (
                    policy.containerMatch.split(',').map((name) => (
                      <Badge key={name} variant="outline" className="text-[10px] px-1.5 py-0">{name.trim()}</Badge>
                    ))
                  )}
                </span>
                <span>{t('updates.imagePattern')}: <code className="font-mono">{policy.imageMatch || '*'}</code></span>
                <span>{t('updates.schedule')}: <code className="font-mono">{policy.schedule}</code></span>
                {policy.autoRestart && <span>{t('updates.autoRestart')}</span>}
                {policy.lastRunAt && (
                  <span>{t('updates.lastRun')}: {timeAgo(policy.lastRunAt)}</span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={policy.enabled ? t('updates.disable') : t('updates.enable')}
                    onClick={() => handleToggle(policy.id, policy.enabled)}
                  >
                    {policy.enabled
                      ? <IconPlayerPause className="size-4" />
                      : <IconPlayerPlay className="size-4" />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{policy.enabled ? t('updates.disable') : t('updates.enable')}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={tc('actions.delete')}
                    className="text-red-500 hover:text-red-400"
                    onClick={() => handleDelete(policy.id)}
                  >
                    <IconTrash className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{tc('actions.delete')}</TooltipContent>
              </Tooltip>
            </div>
          </div>
        ))}
      </div>

      {/* Create dialog (inline) */}
      {showDialog && (
        <div className="rounded-lg border border-border bg-card p-6 space-y-4">
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
            <div>
              <Label className="mb-1.5">{t('updates.scheduleLabel')}</Label>
              <Input
                value={form.schedule}
                onChange={(e) => setForm({ ...form, schedule: e.target.value })}
                placeholder="0 3 * * *"
              />
              <p className="mt-1 text-xs text-muted-foreground">{t('updates.scheduleHint')}</p>
              <CronSchedulePreview expression={form.schedule} className="mt-2 rounded-md border border-border/60 bg-muted/30 p-2" />
            </div>
          </div>

          {/* Container selector — tag style */}
          <div>
            <Label className="mb-1.5">{t('updates.containerMatchLabel')}</Label>
            <div className="mt-1.5 flex flex-wrap gap-1.5 rounded-lg border border-border bg-muted/50 p-2 min-h-[42px]">
              <Badge
                variant={allContainers ? 'default' : 'outline'}
                className="cursor-pointer select-none text-xs px-2 py-0.5"
                onClick={toggleAllContainers}
              >
                {t('updates.allContainers')}
              </Badge>
              {containerNames.map((name) => {
                const selected = selectedContainers.includes(name);
                return (
                  <Badge
                    key={name}
                    variant={selected ? 'default' : 'outline'}
                    className="cursor-pointer select-none text-xs px-2 py-0.5 gap-1"
                    onClick={() => toggleContainer(name)}
                  >
                    {name}
                    {selected && <IconX className="size-3" />}
                  </Badge>
                );
              })}
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
              <Switch
                checked={form.autoRestart}
                onCheckedChange={(v) => setForm({ ...form, autoRestart: v })}
              />
              <Label>{t('updates.autoRestartLabel')}</Label>
            </div>
          </div>

          <div className="flex gap-2 pt-2">
            <Button onClick={handleCreate} disabled={createPolicy.isPending || !form.name.trim()}>
              {createPolicy.isPending ? t('updates.creating') : tc('actions.create')}
            </Button>
            <Button variant="outline" onClick={() => setShowDialog(false)}>
              {tc('actions.cancel')}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
