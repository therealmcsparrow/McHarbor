// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconFolder } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { kindLabel } from '@resources/utils/secret-kind';
import type { Secret } from '@core/types/secrets';
import { InsertSecretBranch } from './InsertSecretBranch';
import type { SecretScopeNode } from './insert-secret-tree';

type InsertSecretTreeProps = {
  nodes: SecretScopeNode[];
  collapsed: readonly string[];
  onToggle: (key: string) => void;
  onSelect: (secret: Secret) => void;
  forceExpanded: boolean;
};

export function InsertSecretTree({ nodes, collapsed, onToggle, onSelect, forceExpanded }: InsertSecretTreeProps) {
  const { t } = useTranslation('secrets');
  const isOpen = (key: string) => forceExpanded || !collapsed.includes(key);

  return (
    <ul className="max-h-80 space-y-0.5 overflow-auto" role="tree">
      {nodes.map((scopeNode) => (
        <InsertSecretBranch
          key={scopeNode.key}
          label={t(`scope.${scopeNode.scope}`)}
          count={scopeNode.count}
          open={isOpen(scopeNode.key)}
          onToggle={() => onToggle(scopeNode.key)}
          className="text-sm font-medium"
        >
          {scopeNode.kinds.map((kindNode) => (
            <InsertSecretBranch
              key={kindNode.key}
              label={kindLabel(t, kindNode.kind)}
              count={kindNode.count}
              open={isOpen(kindNode.key)}
              onToggle={() => onToggle(kindNode.key)}
              className="text-xs font-normal"
            >
              {kindNode.groups.map((groupNode) => (
                <InsertSecretBranch
                  key={groupNode.key}
                  label={groupNode.group || t('groups.ungrouped')}
                  count={groupNode.secrets.length}
                  open={isOpen(groupNode.key)}
                  onToggle={() => onToggle(groupNode.key)}
                  icon={<IconFolder className="size-3.5 shrink-0" />}
                  className="text-xs font-normal text-muted-foreground"
                >
                  {groupNode.secrets.map((secret) => (
                    <li key={secret.id} role="none">
                      <Button
                        variant="ghost"
                        type="button"
                        role="treeitem"
                        aria-selected={false}
                        onClick={() => onSelect(secret)}
                        className="h-auto w-full flex-col items-start gap-0.5 rounded-md border border-transparent px-3 py-1.5 text-left font-normal hover:border-border hover:bg-muted/50 focus:border-border focus:bg-muted/50"
                      >
                        <span className="flex min-w-0 items-center gap-2">
                          <span className="truncate font-mono text-xs font-medium">{secret.name}</span>
                          {secret.stackName && <Badge variant="secondary">{secret.stackName}</Badge>}
                        </span>
                        {secret.description && (
                          <span className="w-full truncate text-xs text-muted-foreground">{secret.description}</span>
                        )}
                      </Button>
                    </li>
                  ))}
                </InsertSecretBranch>
              ))}
            </InsertSecretBranch>
          ))}
        </InsertSecretBranch>
      ))}
    </ul>
  );
}
