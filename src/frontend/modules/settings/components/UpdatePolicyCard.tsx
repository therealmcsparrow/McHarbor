// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBoxMultiple, IconPlayerPause, IconPlayerPlay, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { timeAgo } from '@resources/utils/format';
import { describeCron } from '@resources/utils/schedule';
import { PERM } from '@core/auth/permissionKeys';
import type { UpdatePolicy } from '../hooks/useUpdates';
import { PolicyContainersDialog, parseContainerMatch } from './PolicyContainersDialog';

const SUMMARY_LIMIT = 4;

type Props = {
  policy: UpdatePolicy;
  saving: boolean;
  onToggle: (id: string, enabled: boolean) => void;
  onDelete: (id: string) => void;
  onSaveContainers: (id: string, containerMatch: string) => void;
};

// UpdatePolicyCard renders one auto-update policy. The container summary
// is capped so a policy with many targets does not swallow the row; the
// full list lives in the picker dialog behind the "edit containers" button.
export function UpdatePolicyCard({ policy, saving, onToggle, onDelete, onSaveContainers }: Props) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const [pickerOpen, setPickerOpen] = useState(false);

  const names = parseContainerMatch(policy.containerMatch);
  const visible = names.slice(0, SUMMARY_LIMIT);
  const hidden = names.length - visible.length;

  return (
    <div className="mc-glass flex items-center justify-between rounded-lg p-4">
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
          <span className="flex flex-wrap items-center gap-1">
            {t('updates.containerPattern')}:
            {names.length === 0 ? (
              <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                {t('updates.allContainers')}
              </Badge>
            ) : (
              <>
                {visible.map((policyName) => (
                  <Badge key={policyName} variant="outline" className="px-1.5 py-0 text-[10px]">
                    {policyName}
                  </Badge>
                ))}
                {hidden > 0 && (
                  <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                    {t('updates.moreContainers', { count: hidden })}
                  </Badge>
                )}
              </>
            )}
          </span>
          <span>
            {t('updates.imagePattern')}: <code className="font-mono">{policy.imageMatch || '*'}</code>
          </span>
          <span>
            {t('updates.schedule')}: {describeCron(policy.schedule, tc)}
          </span>
          {policy.autoRestart && <span>{t('updates.autoRestart')}</span>}
          {policy.lastRunAt && (
            <span>
              {t('updates.lastRun')}: {timeAgo(policy.lastRunAt)}
            </span>
          )}
        </div>
      </div>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              permission={PERM.UPDATES_MANAGE}
              variant="ghost"
              size="icon"
              aria-label={t('updates.editContainers')}
              onClick={() => setPickerOpen(true)}
            >
              <IconBoxMultiple className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('updates.editContainers')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              permission={PERM.UPDATES_MANAGE}
              variant="ghost"
              size="icon"
              aria-label={policy.enabled ? t('updates.disable') : t('updates.enable')}
              onClick={() => onToggle(policy.id, policy.enabled)}
            >
              {policy.enabled ? <IconPlayerPause className="size-4" /> : <IconPlayerPlay className="size-4" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{policy.enabled ? t('updates.disable') : t('updates.enable')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              permission={PERM.UPDATES_MANAGE}
              variant="ghost"
              size="icon"
              aria-label={t('actions.delete', { ns: 'common' })}
              className="text-danger hover:text-danger"
              onClick={() => onDelete(policy.id)}
            >
              <IconTrash className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('actions.delete', { ns: 'common' })}</TooltipContent>
        </Tooltip>
      </div>

      <PolicyContainersDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        value={policy.containerMatch}
        saving={saving}
        onSave={(containerMatch) => {
          onSaveContainers(policy.id, containerMatch);
          setPickerOpen(false);
        }}
      />
    </div>
  );
}
