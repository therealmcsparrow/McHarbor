// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconRocket, IconDatabase, IconWorld, IconShield, IconServer, IconMessage,
  IconMail, IconFileCode, IconPhoto, IconChecklist, IconNetwork, IconPlus,
  IconPencil, IconTrash, IconSearch,
} from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
  DialogDescription, DialogFooter,
} from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { useBlueprints, useDeployBlueprint, useDeleteBlueprint, type Blueprint } from '../hooks/useBlueprints';
import { BlueprintFormDialog } from '../components/BlueprintFormDialog';

const CATEGORY_ICONS: Record<string, React.ReactNode> = {
  database: <IconDatabase className="size-8" />,
  web: <IconWorld className="size-8" />,
  security: <IconShield className="size-8" />,
  infrastructure: <IconServer className="size-8" />,
  messaging: <IconMessage className="size-8" />,
  mail: <IconMail className="size-8" />,
  development: <IconFileCode className="size-8" />,
  media: <IconPhoto className="size-8" />,
  productivity: <IconChecklist className="size-8" />,
  networking: <IconNetwork className="size-8" />,
  automation: <IconRocket className="size-8" />,
};

export default function BlueprintsPage() {
  const { t } = useTranslation('common');
  const { data: blueprints = [], isLoading } = useBlueprints();
  const deploy = useDeployBlueprint(t('blueprints.mutationSuccess'));
  const deleteMutation = useDeleteBlueprint(t('blueprints.deleteSuccess'));

  const [search, setSearch] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editBp, setEditBp] = useState<Blueprint | null>(null);
  const [deployOpen, setDeployOpen] = useState(false);
  const [deployBp, setDeployBp] = useState<Blueprint | null>(null);
  const [stackName, setStackName] = useState('');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteBp, setDeleteBp] = useState<Blueprint | null>(null);

  const filtered = useMemo(() => {
    if (!search.trim()) return blueprints;
    const q = search.toLowerCase();
    return blueprints.filter(
      (bp) => bp.name.toLowerCase().includes(q) || bp.description.toLowerCase().includes(q) || bp.category.toLowerCase().includes(q)
    );
  }, [blueprints, search]);

  const handleCreate = () => { setEditBp(null); setFormOpen(true); };
  const handleEdit = (bp: Blueprint) => { setEditBp(bp); setFormOpen(true); };
  const handleDeploy = (bp: Blueprint) => {
    setDeployBp(bp);
    setStackName(bp.name.toLowerCase().replace(/\s+/g, '-'));
    setDeployOpen(true);
  };
  const handleDelete = (bp: Blueprint) => { setDeleteBp(bp); setDeleteOpen(true); };

  const confirmDeploy = () => {
    if (!deployBp || !stackName.trim()) return;
    deploy.mutate({ id: deployBp.id, stackName: stackName.trim() }, {
      onSuccess: () => { setDeployOpen(false); setDeployBp(null); },
    });
  };

  const confirmDelete = () => {
    if (!deleteBp) return;
    deleteMutation.mutate(deleteBp.id, {
      onSuccess: () => { setDeleteOpen(false); setDeleteBp(null); },
    });
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <PageHeader title={t('blueprints.title')} description={t('blueprints.description')} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={`blueprint-skeleton-${i + 1}`} className="h-48 animate-pulse rounded-lg border border-border bg-card" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('blueprints.title')}
        description={t('blueprints.description')}
        actions={
          <Button onClick={handleCreate}>
            <IconPlus className="size-4" /> {t('blueprints.create')}
          </Button>
        }
      />

      <div className="relative max-w-sm">
        <IconSearch className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('blueprints.searchPlaceholder')}
          className="pl-9"
        />
      </div>

      {filtered.length === 0 ? (
        <div className="py-12 text-center text-muted-foreground">
          {search ? t('blueprints.noResults') : t('blueprints.noBlueprints')}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {filtered.map((bp) => (
            <div
              key={bp.id}
              className="group flex flex-col rounded-lg border border-border bg-card p-6 transition-colors hover:border-primary/50"
            >
              <div className="mb-4 flex items-start justify-between">
                <div className="text-muted-foreground">
                  {CATEGORY_ICONS[bp.category] ?? <IconRocket className="size-8" />}
                </div>
                <div className="flex items-center gap-1">
                  <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="size-7" aria-label={t('actions.edit')} onClick={() => handleEdit(bp)}>
                          <IconPencil className="size-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t('actions.edit')}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon" className="size-7 text-destructive" aria-label={t('actions.delete')} onClick={() => handleDelete(bp)}>
                          <IconTrash className="size-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t('actions.delete')}</TooltipContent>
                    </Tooltip>
                  </div>
                  <Badge variant="secondary">{bp.category}</Badge>
                </div>
              </div>
              <h3 className="mb-1 text-lg font-semibold text-foreground">{bp.name}</h3>
              <p className="mb-4 flex-1 text-sm text-muted-foreground line-clamp-2">{bp.description}</p>
              <Button className="w-full" onClick={() => handleDeploy(bp)}>
                <IconRocket className="size-4" /> {t('actions.deploy')}
              </Button>
            </div>
          ))}
        </div>
      )}

      <BlueprintFormDialog open={formOpen} onOpenChange={setFormOpen} blueprint={editBp} />

      <Dialog open={deployOpen} onOpenChange={setDeployOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('blueprints.deployTitle', { name: deployBp?.name })}</DialogTitle>
            <DialogDescription>{deployBp?.description}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label>{t('blueprints.stackNameLabel')}</Label>
            <Input value={stackName} onChange={(e) => setStackName(e.target.value)} placeholder="my-stack" />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeployOpen(false)}>{t('actions.cancel')}</Button>
            <Button onClick={confirmDeploy} disabled={deploy.isPending || !stackName.trim()}>
              {deploy.isPending ? t('blueprints.deploying') : t('actions.deploy')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('blueprints.deleteTitle')}</DialogTitle>
            <DialogDescription>{t('blueprints.deleteConfirm', { name: deleteBp?.name })}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>{t('actions.cancel')}</Button>
            <Button variant="destructive" onClick={confirmDelete} disabled={deleteMutation.isPending}>
              {deleteMutation.isPending ? t('actions.deleting') : t('actions.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

