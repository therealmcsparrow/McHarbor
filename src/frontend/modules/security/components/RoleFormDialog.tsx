// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@resources/components/ui/Dialog';
import { useCreateRole, useUpdateRole, useRolePresets, type RolePreset } from '../hooks/useRoles';
import { PermissionEditor, type PermissionSelection } from '@resources/components/security/PermissionEditor';
import { RolePresetPicker } from './RolePresetPicker';
import { flattenSelection, selectionFromPermissions } from './roleSelection';
import type { Role } from '../hooks/useRoles';
import { PERM } from '@core/auth/permissionKeys';

type RoleViewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role: Role;
};

/**
 * Read-only view of a role. Renders the exact same modal as create/edit
 * so the operator sees permissions in the familiar editor layout — minus
 * the preset picker, with every control disabled.
 */
export function RoleViewDialog({ open: openState, onOpenChange, role }: RoleViewDialogProps) {
  return <RoleFormDialog open={openState} onOpenChange={onOpenChange} role={role} readOnly />;
}

type RoleFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role?: Role;
  /** Render the form as a non-editable viewer (no presets, no submit). */
  readOnly?: boolean;
};

export function RoleFormDialog({ open: isOpen, onOpenChange, role, readOnly = false }: RoleFormDialogProps) {
  const { t } = useTranslation('security');
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const { data: presets = [] } = useRolePresets();
  const [itemName, setName] = useState(role?.name ?? '');
  const [description, setDescription] = useState(role?.description ?? '');
  const [selection, setSelection] = useState<PermissionSelection>(selectionFromPermissions(role?.permissions ?? []));
  const isEdit = !!role && !readOnly;

  // Phase 5: apply a preset to seed the permission selection.
  // Replaces whatever is currently selected; the operator can then
  // fine-tune. System role permissions are immutable, so for edits
  // of system roles this only fills in any missing entries — but
  // since the permission editor is read-only for system roles
  // today, presets are effectively a "create new role" feature.
  const handleApplyPreset = (preset: RolePreset) => {
    if (preset.permissions.includes('*')) {
      setSelection({ overrides: {}, wildcard: true });
    } else {
      const overrides: Record<string, 'grant' | 'deny'> = {};
      for (const p of preset.permissions) overrides[p] = 'grant';
      setSelection({ overrides, wildcard: false });
    }
  };

  const handleSubmit = () => {
    if (readOnly) return;
    const permissions = flattenSelection(selection);
    if (isEdit) {
      updateRole.mutate(
        { id: role.id, name: itemName, description, permissions },
        {
          onSuccess: () => onOpenChange(false),
        },
      );
    } else {
      createRole.mutate(
        { name: itemName, description, permissions },
        {
          onSuccess: () => {
            onOpenChange(false);
            setName('');
            setDescription('');
            setSelection({ overrides: {}, wildcard: false });
          },
        },
      );
    }
  };

  const isPending = createRole.isPending || updateRole.isPending;

  // Compact summary under the editor so the operator can see how
  // many explicit grants/denies the dialog will persist.
  const grantCount = Object.values(selection.overrides).filter((s) => s === 'grant').length;
  const denyCount = Object.values(selection.overrides).filter((s) => s === 'deny').length;

  const title = readOnly && role ? role.name : isEdit ? t('roles.editRole') : t('roles.createRole');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {title}
            {readOnly && role?.isSystem && (
              <Badge variant="secondary">
                <IconLock className="mr-1 size-3" />
                {t('roles.system')}
              </Badge>
            )}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <Input
            placeholder={t('roles.name')}
            aria-label={t('roles.name')}
            value={itemName}
            onChange={(e) => setName(e.target.value)}
            readOnly={readOnly}
          />
          <Input
            placeholder={t('roles.roleDescription')}
            aria-label={t('roles.roleDescription')}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            readOnly={readOnly}
          />

          {!readOnly && presets.length > 0 && <RolePresetPicker presets={presets} onApply={handleApplyPreset} />}

          <div>
            <label className="mb-2 block text-sm font-medium">{t('roles.permissions')}</label>
            <PermissionEditor value={selection} onChange={setSelection} readOnly={readOnly} />
            <TypographyText size="xs" tone="muted" className="mt-2">
              {selection.wildcard
                ? t('permissions.wildcardSummary')
                : t('permissions.summary', {
                    grants: grantCount,
                    denies: denyCount,
                  })}
            </TypographyText>
          </div>
        </div>

        <DialogFooter>
          {readOnly ? (
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('actions.close', { ns: 'common' })}
            </Button>
          ) : (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t('actions.cancel', { ns: 'common' })}
              </Button>
              <Button permission={PERM.ROLES_MANAGE} onClick={handleSubmit} disabled={!itemName || isPending}>
                {isPending
                  ? t('actions.processing', { ns: 'common' })
                  : isEdit
                    ? t('actions.save', { ns: 'common' })
                    : t('actions.create', { ns: 'common' })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
