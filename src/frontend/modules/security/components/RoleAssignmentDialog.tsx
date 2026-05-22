// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { useStacksForEnvironment } from '@resources/hooks/useStacksForEnvironment';
import { Select } from '@resources/components/ui/Select';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { Role } from '../hooks/useRoles';

type ExistingRoleAssignment = {
  roleId: string;
  environmentId: string | null;
  stackName: string | null;
};

type RoleAssignmentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roles: Role[];
  existingAssignments?: ExistingRoleAssignment[];
  onAssign: (roleId: string, environmentId?: string, stackNames?: string[]) => void;
};

export function RoleAssignmentDialog({
  open,
  onOpenChange,
  roles,
  existingAssignments = [],
  onAssign,
}: RoleAssignmentDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);
  const [roleId, setRoleId] = useState('');
  const [envId, setEnvId] = useState('');
  const [stackFilter, setStackFilter] = useState('');
  const [selectedStackNames, setSelectedStackNames] = useState<string[]>([]);
  const { data: stacks = [], isLoading: isStacksLoading } = useStacksForEnvironment(envId, open && !!envId);

  const roleOptions = roles.map((role) => ({ value: role.id, label: role.name }));
  const envOptions = [
    { value: '', label: t('users.selectEnvironment') },
    ...environments.map((environment) => ({ value: environment.id, label: environment.name })),
  ];

  const stackNames = useMemo(
    () =>
      Array.from(new Set(stacks.map((stack) => stack.name))).sort((left, right) => left.localeCompare(right)),
    [stacks],
  );
  const filteredStackNames = useMemo(() => {
    const query = stackFilter.trim().toLowerCase();
    if (!query) {
      return stackNames;
    }

    return stackNames.filter((stackName) => stackName.toLowerCase().includes(query));
  }, [stackFilter, stackNames]);
  const assignedStackNames = useMemo(() => {
    if (!roleId || !envId) {
      return new Set<string>();
    }

    return new Set(
      existingAssignments
        .filter(
          (assignment) =>
            assignment.roleId === roleId &&
            assignment.environmentId === envId &&
            assignment.stackName !== null,
        )
        .map((assignment) => assignment.stackName as string),
    );
  }, [envId, existingAssignments, roleId]);
  const availableFilteredStackNames = useMemo(
    () => filteredStackNames.filter((stackName) => !assignedStackNames.has(stackName)),
    [assignedStackNames, filteredStackNames],
  );

  const hasGlobalAssignment = !!roleId && existingAssignments.some(
    (assignment) => assignment.roleId === roleId && assignment.environmentId === null && assignment.stackName === null,
  );
  const hasEnvironmentAssignment = !!roleId && !!envId && existingAssignments.some(
    (assignment) => assignment.roleId === roleId && assignment.environmentId === envId && assignment.stackName === null,
  );

  useEffect(() => {
    setStackFilter('');
    setSelectedStackNames([]);
  }, [envId, roleId]);

  useEffect(() => {
    if (open) {
      return;
    }

    setRoleId('');
    setEnvId('');
    setStackFilter('');
    setSelectedStackNames([]);
  }, [open]);

  const duplicateScopeMessage = useMemo(() => {
    if (!roleId) {
      return '';
    }
    if (hasGlobalAssignment) {
      return t('users.roleAlreadyAssignedGlobal');
    }
    if (envId && hasEnvironmentAssignment) {
      return t('users.roleAlreadyAssignedEnvironment');
    }

    return '';
  }, [envId, hasEnvironmentAssignment, hasGlobalAssignment, roleId, t]);

  const handleToggleStack = (stackName: string) => {
    setSelectedStackNames((current) =>
      current.includes(stackName)
        ? current.filter((item) => item !== stackName)
        : [...current, stackName],
    );
  };

  const handleSelectVisible = () => {
    setSelectedStackNames((current) =>
      Array.from(new Set([...current, ...availableFilteredStackNames])),
    );
  };

  const handleSubmit = () => {
    if (!roleId || duplicateScopeMessage) {
      return;
    }

    onAssign(roleId, envId || undefined, selectedStackNames.length > 0 ? selectedStackNames : undefined);
    setRoleId('');
    setEnvId('');
    setStackFilter('');
    setSelectedStackNames([]);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('users.assignRole')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <label className="mb-1 block text-sm font-medium">{t('users.selectRole')}</label>
            <Select
              value={roleId}
              onChange={setRoleId}
              options={roleOptions}
              placeholder={t('users.selectRole')}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">{t('users.environment')}</label>
            <Select value={envId} onChange={setEnvId} options={envOptions} />
          </div>

          {duplicateScopeMessage ? (
            <p className="rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
              {duplicateScopeMessage}
            </p>
          ) : null}

          {envId ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <label className="text-sm font-medium">{tc('nav.stacks')}</label>
                <span className="text-xs text-muted-foreground">
                  {t('users.selectedStacksCount', { count: selectedStackNames.length })}
                </span>
              </div>

              <Input
                value={stackFilter}
                onChange={(event) => setStackFilter(event.target.value)}
                placeholder={tc('select.searchPlaceholder')}
                variant="outline"
              />

              <div className="flex items-center justify-between gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleSelectVisible}
                  disabled={availableFilteredStackNames.length === 0}
                >
                  {t('users.selectVisibleStacks')}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setSelectedStackNames([])}
                  disabled={selectedStackNames.length === 0}
                >
                  {tc('actions.clear')}
                </Button>
              </div>

              <div className="max-h-56 space-y-2 overflow-y-auto rounded-lg border border-border p-3">
                {isStacksLoading ? (
                  <p className="text-sm text-muted-foreground">{tc('dataGrid.loading')}</p>
                ) : stackNames.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{t('users.noStacksInEnvironment')}</p>
                ) : availableFilteredStackNames.length === 0 ? (
                  <p className="text-sm text-muted-foreground">{t('users.noStacksMatch')}</p>
                ) : (
                  availableFilteredStackNames.map((stackName) => (
                    <label key={stackName} className="flex cursor-pointer items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/30">
                      <Checkbox
                        checked={selectedStackNames.includes(stackName)}
                        onCheckedChange={() => handleToggleStack(stackName)}
                      />
                      <span className="text-sm text-foreground">{stackName}</span>
                    </label>
                  ))
                )}
              </div>

              <p className="text-xs text-muted-foreground">{t('users.stackScopeHint')}</p>
            </div>
          ) : null}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!roleId || !!duplicateScopeMessage}>
            {t('users.assignRole')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
