// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useAgents } from '../hooks/useAgentSettings';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  excluded: string[];
  saving: boolean;
  onSave: (envIds: string[]) => void;
};

// AgentUpdateExceptionsDialog lets the operator pick which agent
// environments an update sweep must leave alone. It edits a local draft
// and only writes on Save so a stray click never persists half a list.
export function AgentUpdateExceptionsDialog({ open: isOpen, onOpenChange, excluded, saving, onSave }: Props) {
  const { t } = useTranslation('settings');
  const { data: agents = [], isLoading, isError, error } = useAgents();
  const [draft, setDraft] = useState<string[]>(excluded);

  const [seeded, setSeeded] = useState({ isOpen, excluded });
  if (seeded.isOpen !== isOpen || seeded.excluded !== excluded) {
    setSeeded({ isOpen, excluded });
    if (isOpen) setDraft(excluded);
  }

  // Keep ids that no longer match a known agent visible so they can be
  // removed instead of silently lingering in the saved list.
  const knownIds = new Set(agents.map((a) => a.envId));
  const orphanIds = draft.filter((id) => !knownIds.has(id));

  function toggle(envId: string, checked: boolean) {
    setDraft((prev) => (checked ? [...prev, envId] : prev.filter((id) => id !== envId)));
  }

  return (
    <Dialog open={isOpen} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('about.autoUpdate.exceptions.title')}</DialogTitle>
          <DialogDescription>{t('about.autoUpdate.exceptions.description')}</DialogDescription>
        </DialogHeader>

        <div className="max-h-80 space-y-1.5 overflow-y-auto">
          {isLoading && (
            <div className="flex justify-center py-6">
              <Spinner size="sm" />
            </div>
          )}
          {isError && <ErrorState variant="inline" title={t('about.autoUpdate.exceptions.loadFailed')} error={error} />}
          {!isLoading && !isError && agents.length === 0 && orphanIds.length === 0 && (
            <p className="rounded-md border border-dashed border-border bg-muted/30 p-3 text-xs text-muted-foreground">
              {t('about.autoUpdate.exceptions.empty')}
            </p>
          )}
          {agents.map((agent) => {
            const checked = draft.includes(agent.envId);
            return (
              <label
                key={agent.envId}
                className="mc-glass flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm"
              >
                <Checkbox
                  checked={checked}
                  onCheckedChange={(v) => toggle(agent.envId, v === true)}
                  disabled={saving}
                  aria-label={t('about.autoUpdate.exceptions.toggleAria', { name: agent.envName })}
                />
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium text-foreground">{agent.envName}</div>
                  <div className="truncate text-xs text-muted-foreground">
                    {agent.hostname || agent.envId}
                    {agent.agentVersion ? ` · v${agent.agentVersion}` : ''}
                  </div>
                </div>
                <Badge variant={agent.status === 'connected' ? 'default' : 'secondary'}>{agent.status}</Badge>
                {checked && <Badge variant="outline">{t('about.autoUpdate.exceptions.excludedBadge')}</Badge>}
              </label>
            );
          })}
          {orphanIds.map((id) => (
            <label key={id} className="mc-glass flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm">
              <Checkbox
                checked
                onCheckedChange={(v) => toggle(id, v === true)}
                disabled={saving}
                aria-label={t('about.autoUpdate.exceptions.toggleAria', { name: id })}
              />
              <div className="min-w-0 flex-1 truncate text-muted-foreground">
                {t('about.autoUpdate.exceptions.unknownAgent', { id })}
              </div>
            </label>
          ))}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t('about.autoUpdate.exceptions.cancel')}
          </Button>
          <Button permission={PERM.ENVIRONMENTS_MANAGE} onClick={() => onSave(draft)} disabled={saving}>
            {saving ? <Spinner size="sm" /> : null}
            {t('about.autoUpdate.exceptions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
