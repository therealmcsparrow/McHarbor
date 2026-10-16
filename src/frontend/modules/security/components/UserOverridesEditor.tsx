// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Per-user grant/deny permission overrides.
//
// This was a modal launched from the users rail. It is a form with a
// per-row advanced-scope disclosure and a list that grows without bound —
// none of which belongs in a dialog sized to a viewport it does not own.
// It is now a tab panel on the user detail page, with the save action
// pinned to the bottom of the panel rather than a dialog footer.
//
// user_permissions (migrations/070_rbac_v2.sql) stores the entries;
// PUT /users/{id}/overrides atomically replaces the user's full list, so
// the draft below is always the complete set, never a delta.

import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { usePermissionCatalog } from '@core/auth/usePermission';
import { useUserOverrides, useSetUserOverrides } from '@resources/hooks/useUsers';
import { useOverridesDraft } from '../hooks/useOverridesDraft';
import { OverrideRow } from './OverrideRow';

type UserOverridesEditorProps = {
  userId: string;
};

export function UserOverridesEditor({ userId }: UserOverridesEditorProps) {
  const { t } = useTranslation('security');
  // Cancel/Save are shared strings; the `security` namespace has no
  // `actions` block, so the dialog these rows came from was rendering the
  // raw keys.
  const { data: overrides = [], isLoading } = useUserOverrides(userId);
  // The permission catalog lives at /auth/permissions/catalog and already has
  // a hook. This used to fetch /rbac/policies/permissions/catalog, which is not
  // a route: chi matched it as /rbac/policies/{id} with id="permissions" and
  // then had no /catalog under it, so the editor rendered an empty picker.
  const { data: catalog = [] } = usePermissionCatalog();
  const setOverrides = useSetUserOverrides();
  const { draft, dirty, addRow, removeRow, updateRow, reset, toPayload } = useOverridesDraft(overrides);

  function save() {
    setOverrides.mutate({ userId, overrides: toPayload() });
  }

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t('users.overrides.hint')}</p>
        <Button
          permission={PERM.PERMISSIONS_ASSIGN}
          variant="outline"
          size="sm"
          onClick={addRow}
          data-testid="add-override"
        >
          <IconPlus className="mr-1 size-3.5" />
          {t('users.overrides.addOverride')}
        </Button>
      </div>

      {draft.length === 0 && (
        <p className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
          {t('users.overrides.empty')}
        </p>
      )}

      {draft.map((row, index) => (
        <OverrideRow
          key={row.uid}
          row={row}
          catalog={catalog}
          onChange={(key, value) => updateRow(index, key, value)}
          onRemove={() => removeRow(index)}
        />
      ))}

      <div className="flex justify-end gap-2 border-t border-border pt-3">
        <Button variant="outline" onClick={reset} disabled={!dirty || setOverrides.isPending}>
          {t('actions.cancel', { ns: 'common' })}
        </Button>
        <Button
          permission={PERM.PERMISSIONS_ASSIGN}
          onClick={save}
          disabled={!dirty || setOverrides.isPending}
          data-testid="save-overrides"
        >
          {setOverrides.isPending ? <Spinner size="sm" /> : t('actions.save', { ns: 'common' })}
        </Button>
      </div>
    </div>
  );
}
