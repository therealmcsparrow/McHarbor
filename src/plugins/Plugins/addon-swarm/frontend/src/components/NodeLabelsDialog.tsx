// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Button } from '@resources/components/ui/Button';
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
import { Spinner } from '@resources/components/ui/Spinner';
import { useSetNodeLabels } from '../hooks/useSwarmNodes';
import type { SwarmNode } from '../types';

type LabelRow = { id: string; key: string; value: string };

const newRow = (key = '', value = ''): LabelRow => ({ id: crypto.randomUUID(), key, value });

type NodeLabelsDialogProps = {
  node: SwarmNode | null;
  onOpenChange: (open: boolean) => void;
};

/**
 * Edit a node's labels.
 *
 * Labels are what placement constraints match on, so this is the control that
 * decides where work can land. It edits the whole set at once because that is
 * what the API takes — a partial update would silently drop the labels the
 * form did not know about.
 */
export function NodeLabelsDialog({ node, onOpenChange }: NodeLabelsDialogProps) {
  const { t } = useTranslation('swarm');
  const setLabels = useSetNodeLabels();
  const [rows, setRows] = useState<LabelRow[]>([]);

  const [seenNode, setSeenNode] = useState(node);
  if (seenNode !== node) {
    setSeenNode(node);
    if (node) {
      const entries = Object.entries(node.labels ?? {}).map(([key, value]) => newRow(key, value));
      setRows(entries.length > 0 ? entries : [newRow()]);
    }
  }

  const update = (index: number, patch: Partial<LabelRow>) => {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  };

  const submit = () => {
    if (!node) return;
    const labels: Record<string, string> = {};
    for (const row of rows) {
      const key = row.key.trim();
      if (key) labels[key] = row.value;
    }
    setLabels.mutate({ id: node.id, labels }, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={node !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('dialogs.nodeLabels.title')}</DialogTitle>
          <DialogDescription>{t('dialogs.nodeLabels.description', { name: node?.hostname ?? '' })}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-2">
          {rows.map((row, index) => (
            <div key={row.id} className="flex items-center gap-2">
              <Input
                value={row.key}
                onChange={(e) => update(index, { key: e.target.value })}
                placeholder={t('fields.labelKey')}
                aria-label={t('fields.labelKey')}
              />
              <Input
                value={row.value}
                onChange={(e) => update(index, { value: e.target.value })}
                placeholder={t('fields.labelValue')}
                aria-label={t('fields.labelValue')}
              />
              <Button
                variant="ghost"
                size="icon"
                aria-label={t('actions.removeLabel')}
                onClick={() => setRows((current) => current.filter((_, i) => i !== index))}
              >
                <IconTrash className="size-4" />
              </Button>
            </div>
          ))}
          <Button variant="ghost" size="sm" onClick={() => setRows((current) => [...current, newRow()])}>
            <IconPlus className="size-4" />
            {t('actions.addLabel')}
          </Button>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button permission={PERM.SWARM_NODES_MANAGE} onClick={submit} disabled={setLabels.isPending}>
            {setLabels.isPending && <Spinner size="sm" />}
            {t('actions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
