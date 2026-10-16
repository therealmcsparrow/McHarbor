// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconSearch } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusBadge } from '@resources/components/ui/StatusBadge';
import { Switch } from '@resources/components/ui/Switch';
import { cn } from '@resources/utils/cn';
import { PERM } from '@core/auth/permissionKeys';
import { usePolicyContainerDraft } from '../hooks/usePolicyContainerDraft';

export { ALL_CONTAINERS, parseContainerMatch, serializeContainerMatch } from '../hooks/usePolicyContainerDraft';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Current stored `containerMatch` value. */
  value: string;
  saving?: boolean;
  onSave: (containerMatch: string) => void;
};

// PolicyContainersDialog is the picker for which containers an update
// policy applies to. It edits a local draft and only reports on Save so
// a stray click never persists a half-edited list. Names that are no
// longer running (or hand-typed glob patterns) stay visible so they can
// be removed instead of silently lingering in the saved value.
export function PolicyContainersDialog({ open: isOpen, onOpenChange, value, saving = false, onSave }: Props) {
  const { t } = useTranslation('settings');
  const {
    isLoading,
    all,
    setAll,
    draft,
    search,
    setSearch,
    rows,
    orphans,
    filtered,
    filteredOrphans,
    candidate,
    canAddPattern,
    toggle,
    addPattern,
    canSave,
    result,
  } = usePolicyContainerDraft(isOpen, value);

  const handleSave = () => {
    if (canSave) onSave(result);
  };

  return (
    <Dialog open={isOpen} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('updates.containersDialogTitle')}</DialogTitle>
          <DialogDescription>{t('updates.containersDialogDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          <div className="mc-glass flex items-center justify-between gap-3 rounded-md px-3 py-2">
            <div className="min-w-0">
              <Label>{t('updates.applyToAll')}</Label>
              <p className="text-xs text-muted-foreground">{t('updates.applyToAllHint')}</p>
            </div>
            <Switch checked={all} onCheckedChange={setAll} disabled={saving} aria-label={t('updates.applyToAll')} />
          </div>

          <div className="relative">
            <IconSearch className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && canAddPattern) {
                  e.preventDefault();
                  addPattern();
                }
              }}
              placeholder={t('updates.searchContainers')}
              disabled={all || saving}
              aria-label={t('updates.searchContainers')}
              className="pl-8"
            />
          </div>

          <div className={cn('max-h-72 space-y-1.5 overflow-y-auto', all && 'pointer-events-none opacity-50')}>
            {isLoading && (
              <div className="flex justify-center py-6">
                <Spinner size="sm" />
              </div>
            )}
            {!isLoading && canAddPattern && (
              <Button
                variant="outline"
                size="sm"
                className="w-full justify-start"
                onClick={addPattern}
                disabled={all || saving}
              >
                <IconPlus className="size-4" />
                {t('updates.addPattern', { value: candidate })}
              </Button>
            )}
            {!isLoading && rows.length === 0 && orphans.length === 0 && (
              <p className="rounded-md border border-dashed border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                {t('updates.noContainers')}
              </p>
            )}
            {!isLoading &&
              rows.length > 0 &&
              filtered.length === 0 &&
              filteredOrphans.length === 0 &&
              !canAddPattern && (
                <p className="rounded-md border border-dashed border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                  {t('updates.noContainersMatch')}
                </p>
              )}
            {filtered.map((row) => {
              const checked = draft.includes(row.name);
              return (
                <label
                  key={row.name}
                  htmlFor={`policy-container-${row.name}`}
                  className="mc-glass flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm"
                >
                  <Checkbox
                    id={`policy-container-${row.name}`}
                    checked={checked}
                    onCheckedChange={(v) => toggle(row.name, v === true)}
                    disabled={all || saving}
                    aria-label={row.name}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium text-foreground">{row.name}</div>
                    <div className="truncate text-xs text-muted-foreground">{row.image}</div>
                  </div>
                  <StatusBadge status={row.state} />
                </label>
              );
            })}
            {filteredOrphans.map((entryName) => (
              <label
                key={entryName}
                className="mc-glass flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm"
              >
                <Checkbox
                  checked
                  onCheckedChange={(v) => toggle(entryName, v === true)}
                  disabled={all || saving}
                  aria-label={entryName}
                />
                <div className="min-w-0 flex-1 truncate font-mono text-foreground">{entryName}</div>
                <Badge variant="outline">
                  {/[*?[]/.test(entryName) ? t('updates.patternBadge') : t('updates.notFoundBadge')}
                </Badge>
              </label>
            ))}
          </div>

          <p className="text-xs text-muted-foreground">
            {all
              ? t('updates.allContainers')
              : draft.length === 0
                ? t('updates.noneSelected')
                : t('updates.selectedCount', { count: draft.length })}
          </p>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.UPDATES_MANAGE} onClick={handleSave} disabled={saving || !canSave}>
            {saving ? <Spinner size="sm" /> : null}
            {t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
