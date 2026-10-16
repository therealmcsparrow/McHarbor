// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useStacksForEnvironment } from '@resources/hooks/useStacksForEnvironment';
import { Select } from '@resources/components/ui/Select';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { Role } from '../hooks/useRoles';
import { RoleAssignmentStackPicker } from './RoleAssignmentStackPicker';
import { PERM } from '@core/auth/permissionKeys';

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
  open: isOpen,
  onOpenChange,
  roles,
  existingAssignments = [],
  onAssign,
}: RoleAssignmentDialogProps) {
  const { t } = useTranslation('security');
  const environments = useEnvironmentStore((s) => s.environments);
  const [roleId, setRoleId] = useState('');
  const [envId, setEnvId] = useState('');
  const [stackFilter, setStackFilter] = useState('');
  const [selectedStackNames, setSelectedStackNames] = useState<string[]>([]);
  const { data: stacks = [], isLoading: isStacksLoading } = useStacksForEnvironment(envId, isOpen && !!envId);

  const roleOptions = roles.map((role) => ({ value: role.id, label: role.name }));
  const envOptions = [
    { value: '', label: t('users.selectEnvironment') },
    ...environments.map((environment) => ({ value: environment.id, label: environment.name })),
  ];

  const stackNames = useMemo(
    () => Array.from(new Set(stacks.map((stack) => stack.name))).sort((left, right) => left.localeCompare(right)),
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
            assignment.roleId === roleId && assignment.environmentId === envId && assignment.stackName !== null,
        )
        .map((assignment) => assignment.stackName as string),
    );
  }, [envId, existingAssignments, roleId]);
  const availableFilteredStackNames = useMemo(
    () => filteredStackNames.filter((stackName) => !assignedStackNames.has(stackName)),
    [assignedStackNames, filteredStackNames],
  );

  const hasGlobalAssignment =
    !!roleId &&
    existingAssignments.some(
      (assignment) =>
        assignment.roleId === roleId && assignment.environmentId === null && assignment.stackName === null,
    );
  const hasEnvironmentAssignment =
    !!roleId &&
    !!envId &&
    existingAssignments.some(
      (assignment) =>
        assignment.roleId === roleId && assignment.environmentId === envId && assignment.stackName === null,
    );

  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (!isOpen) {
      setRoleId('');
      setEnvId('');
      setStackFilter('');
      setSelectedStackNames([]);
    }
  }

  const pickRole = (next: string) => {
    setRoleId(next);
    setStackFilter('');
    setSelectedStackNames([]);
  };
  const pickEnv = (next: string) => {
    setEnvId(next);
    setStackFilter('');
    setSelectedStackNames([]);
  };

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
      current.includes(stackName) ? current.filter((item) => item !== stackName) : [...current, stackName],
    );
  };

  const handleSelectVisible = () => {
    setSelectedStackNames((current) => Array.from(new Set([...current, ...availableFilteredStackNames])));
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
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('users.assignRole')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <label className="mb-1 block text-sm font-medium">{t('users.selectRole')}</label>
            <Select value={roleId} onChange={pickRole} options={roleOptions} placeholder={t('users.selectRole')} />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">{t('users.environment')}</label>
            <Select value={envId} onChange={pickEnv} options={envOptions} />
          </div>

          {duplicateScopeMessage ? (
            <p className="rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
              {duplicateScopeMessage}
            </p>
          ) : null}

          {envId ? (
            <RoleAssignmentStackPicker
              selectedStackNames={selectedStackNames}
              stackFilter={stackFilter}
              onStackFilterChange={setStackFilter}
              availableStackNames={availableFilteredStackNames}
              hasStacks={stackNames.length > 0}
              isLoading={isStacksLoading}
              onToggle={handleToggleStack}
              onSelectVisible={handleSelectVisible}
              onClear={() => setSelectedStackNames([])}
            />
          ) : null}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.PERMISSIONS_ASSIGN}
            onClick={handleSubmit}
            disabled={!roleId || !!duplicateScopeMessage}
          >
            {t('users.assignRole')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
