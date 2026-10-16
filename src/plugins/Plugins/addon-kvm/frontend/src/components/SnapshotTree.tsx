// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import type { Snapshot, VM } from '../types';
import { SnapshotRow } from './SnapshotTreeRow';
import { CurrentState, Rails, ToggleButton } from './SnapshotTreeParts';

export interface SnapshotNode {
  snapshot: Snapshot;
  children: SnapshotNode[];
}

export function buildSnapshotTree(snapshots: Snapshot[]): SnapshotNode[] {
  const nodes = new Map<string, SnapshotNode>();
  for (const snapshot of snapshots) nodes.set(snapshot.id, { snapshot, children: [] });

  const roots: SnapshotNode[] = [];
  for (const snapshot of snapshots) {
    const node = nodes.get(snapshot.id)!;
    const parentNode = snapshot.parentId ? nodes.get(snapshot.parentId) : undefined;
    if (parentNode) parentNode.children.push(node);
    else roots.push(node);
  }
  return roots;
}

export function SnapshotTree({
  vm,
  snapshots,
  onRevert,
  onDelete,
}: {
  vm: VM;
  snapshots: Snapshot[];
  onRevert: (snapshot: Snapshot) => void;
  onDelete: (snapshot: Snapshot) => void;
}) {
  const { t } = useTranslation('vm');
  const roots = useMemo(() => buildSnapshotTree(snapshots), [snapshots]);
  const currentId = vm.currentSnapshotId ?? '';
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set<string>());

  const parents = useMemo(
    () => new Set(snapshots.filter((entry) => entry.parentId).map((entry) => entry.parentId!)),
    [snapshots],
  );

  function toggle(id: string): void {
    setCollapsed((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }

  return (
    <div className="space-y-2">
      {parents.size > 0 && (
        <div className="flex justify-end gap-1">
          <Button variant="ghost" size="sm" onClick={() => setCollapsed(new Set<string>())}>
            {t('snapshots.tree.expandAll')}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setCollapsed(new Set(parents))}>
            {t('snapshots.tree.collapseAll')}
          </Button>
        </div>
      )}

      <ul className="space-y-1.5">
        {roots.map((node, index) => (
          <Branch
            key={node.snapshot.id}
            node={node}
            vm={vm}
            currentId={currentId}
            rails={[]}
            isLast={index === roots.length - 1 && currentId !== ''}
            collapsed={collapsed}
            onToggle={toggle}
            onRevert={onRevert}
            onDelete={onDelete}
          />
        ))}
        {currentId === '' && <CurrentState vm={vm} rails={[]} />}
      </ul>
    </div>
  );
}

function descendantCount(node: SnapshotNode): number {
  return node.children.reduce((total, child) => total + 1 + descendantCount(child), 0);
}

function Branch({
  node,
  vm,
  currentId,
  rails,
  isLast,
  collapsed,
  onToggle,
  onRevert,
  onDelete,
}: {
  node: SnapshotNode;
  vm: VM;
  currentId: string;
  rails: boolean[];
  isLast: boolean;
  collapsed: ReadonlySet<string>;
  onToggle: (id: string) => void;
  onRevert: (snapshot: Snapshot) => void;
  onDelete: (snapshot: Snapshot) => void;
}) {
  const isCurrent = node.snapshot.id === currentId;
  const childRails = [...rails, !isLast];
  const hasChildren = node.children.length > 0 || isCurrent;
  const isCollapsed = hasChildren && collapsed.has(node.snapshot.id);

  return (
    <li>
      <div className="flex items-stretch">
        <Rails rails={rails} isLast={isLast} />
        {hasChildren ? (
          <ToggleButton expanded={!isCollapsed} name={node.snapshot.name} onClick={() => onToggle(node.snapshot.id)} />
        ) : (
          <span className="w-7 shrink-0" aria-hidden="true" />
        )}
        <SnapshotRow
          vm={vm}
          snapshot={node.snapshot}
          isCurrent={isCurrent}
          hiddenCount={isCollapsed ? descendantCount(node) : 0}
          onRevert={onRevert}
          onDelete={onDelete}
        />
      </div>

      {hasChildren && !isCollapsed && (
        <ul className="space-y-1.5 pt-1.5">
          {node.children.map((child, index) => (
            <Branch
              key={child.snapshot.id}
              node={child}
              vm={vm}
              currentId={currentId}
              rails={childRails}
              isLast={index === node.children.length - 1 && !isCurrent}
              collapsed={collapsed}
              onToggle={onToggle}
              onRevert={onRevert}
              onDelete={onDelete}
            />
          ))}
          {isCurrent && <CurrentState vm={vm} rails={childRails} />}
        </ul>
      )}
    </li>
  );
}
