// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';

type InsertSecretBranchProps = {
  label: string;
  count: number;
  open: boolean;
  onToggle: () => void;
  icon?: ReactNode;
  className?: string;
  children: ReactNode;
};

export function InsertSecretBranch({
  label,
  count,
  open: isOpen,
  onToggle,
  icon,
  className,
  children,
}: InsertSecretBranchProps) {
  const { t } = useTranslation('secrets');
  const Chevron = isOpen ? IconChevronDown : IconChevronRight;

  return (
    <li role="treeitem" aria-expanded={isOpen}>
      <Button
        variant="ghost"
        type="button"
        onClick={onToggle}
        aria-label={isOpen ? t('insert.collapse', { name: label }) : t('insert.expand', { name: label })}
        className={cn('h-auto w-full justify-start gap-2 rounded-md px-2 py-1.5 text-left', className)}
      >
        <Chevron className="size-4 shrink-0 text-muted-foreground" />
        {icon}
        <span className="truncate">{label}</span>
        <Badge variant="secondary">{count}</Badge>
      </Button>

      {isOpen && <ul className="ml-3 border-l border-border pl-2">{children}</ul>}
    </li>
  );
}
