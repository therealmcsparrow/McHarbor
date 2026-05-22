// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { IconLock } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useGroups, useCreateGroup, useDeleteGroup } from '../hooks/useGroups';
import { GroupDetail } from './GroupDetail';

export function GroupsTab() {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const { data: groups, isLoading } = useGroups();
  const [selectedId, setSelectedId] = useState<string>('');
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const createGroup = useCreateGroup();
  const deleteGroup = useDeleteGroup();
  const [newName, setNewName] = useState('');
  const [newDesc, setNewDesc] = useState('');

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-sm font-medium text-muted-foreground">{t('groups.title')}</h3>
          <Button variant="outline" size="sm" onClick={() => setCreateOpen(true)}>
            <IconPlus className="mr-1 size-3.5" />
            {tc('actions.create')}
          </Button>
        </div>

        {groups && groups.length > 0 ? (
          <ul className="space-y-1">
            {groups.map((group) => (
              <li key={group.id}>
                <Button
                  variant="ghost"
                  onClick={() => setSelectedId(group.id)}
                  className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-muted/50 ${
                    selectedId === group.id ? 'bg-primary/10 text-primary' : ''
                  }`}
                >
                  <span className="flex items-center gap-1.5 font-medium">
                    {group.isSystem && <IconLock className="size-3.5 text-muted-foreground" />}
                    {group.name}
                  </span>
                  <Badge variant="secondary">{group.memberCount}</Badge>
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">{t('groups.noGroups')}</p>
        )}
      </div>

      <div className="lg:col-span-2">
        {selectedId ? (
          <GroupDetail groupId={selectedId} onDelete={() => { setDeleteId(selectedId); }} />
        ) : (
          <div className="rounded-lg border border-dashed border-border p-12 text-center text-sm text-muted-foreground">
            {t('groups.description')}
          </div>
        )}
      </div>

      {/* Create dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('groups.createGroup')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <Input placeholder={t('groups.name')} value={newName} onChange={(e) => setNewName(e.target.value)} />
            <Input placeholder={t('groups.groupDescription')} value={newDesc} onChange={(e) => setNewDesc(e.target.value)} />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>{tc('actions.cancel')}</Button>
            <Button
              onClick={() => {
                createGroup.mutate({ name: newName, description: newDesc }, {
                  onSuccess: () => { setCreateOpen(false); setNewName(''); setNewDesc(''); },
                });
              }}
              disabled={!newName || createGroup.isPending}
            >
              {tc('actions.create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirm */}
      <ConfirmDialog
        open={!!deleteId}
        onOpenChange={() => setDeleteId(null)}
        title={t('groups.deleteGroup')}
        description={t('groups.deleteGroupConfirm')}
        onConfirm={() => {
          if (deleteId) {
            deleteGroup.mutate(deleteId, {
              onSuccess: () => { setDeleteId(null); setSelectedId(''); },
            });
          }
        }}
        loading={deleteGroup.isPending}
      />
    </div>
  );
}

