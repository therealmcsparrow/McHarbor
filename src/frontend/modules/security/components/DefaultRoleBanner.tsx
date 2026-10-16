// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { IconShieldCheck } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { api, type ApiResponse } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { Role } from '../hooks/useRoles';

const DEFAULT_ROLE_KEY = 'auth.default_user_role_id';

type Setting = { id?: string; key: string; value: string; category?: string; updatedAt?: string };

function useDefaultRoleSetting() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ['settings', 'by-key', DEFAULT_ROLE_KEY],
    queryFn: () => api.get<Setting>(`/settings/${DEFAULT_ROLE_KEY}`).then(assertSuccess),
  });
  const mutation = useMutation({
    mutationFn: (value: string) => {
      const body: ApiResponse<unknown> = {
        success: true,
        data: { key: DEFAULT_ROLE_KEY, value, category: 'auth' },
      };
      return api.put(`/settings/${DEFAULT_ROLE_KEY}`, body).then(assertSuccess);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'by-key', DEFAULT_ROLE_KEY] });
    },
  });
  return {
    currentRoleId: query.data?.value ?? '',
    isLoading: query.isLoading,
    save: mutation.mutate,
    isSaving: mutation.isPending,
  };
}

type DefaultRoleBannerProps = {
  roles: Role[];
};

/**
 * Default role for new local users (Phase 1 of RBAC roadmap).
 * NULL = refuse new local user creation. We surface a Select over the
 * assignable roles so the operator can pick a known id. Custom roles are
 * listed; the Admin system role is intentionally NOT in the list because
 * assigning Admin to every new local user is too generous — the operator
 * should explicitly grant it.
 *
 * It rides in TablePage's `banner` slot: a page-level notice that has to
 * sit above the filter band rather than below the table.
 */
export function DefaultRoleBanner({ roles }: DefaultRoleBannerProps) {
  const { t } = useTranslation('security');
  const defaultRole = useDefaultRoleSetting();
  const [draftDefaultRoleId, setDraftDefaultRoleId] = useState<string>('');

  // Sync the draft from upstream on first load. After the user starts
  // editing, the effect is a no-op so their typing is never clobbered
  // by a refetch.
  if (!defaultRole.isLoading && draftDefaultRoleId === '' && defaultRole.currentRoleId !== '') {
    setDraftDefaultRoleId(defaultRole.currentRoleId);
  }

  return (
    <div className="mc-glass rounded-lg p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <IconShieldCheck className="mt-0.5 size-5 shrink-0 text-primary" />
          <div>
            <h3 className="text-sm font-semibold text-foreground">{t('roles.defaultRole.title')}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{t('roles.defaultRole.description')}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Select
            value={draftDefaultRoleId}
            onChange={setDraftDefaultRoleId}
            options={roles
              .filter((r) => !r.isSystem || r.id === 'role_viewer')
              .map((r) => ({ value: r.id, label: r.name }))}
            placeholder={t('roles.defaultRole.placeholder')}
            ariaLabel={t('roles.defaultRole.placeholder')}
            variant="outline"
            className="w-56"
            disabled={defaultRole.isLoading || defaultRole.isSaving}
          />
          <Button
            size="sm"
            onClick={() => {
              if (draftDefaultRoleId === defaultRole.currentRoleId) return;
              defaultRole.save(draftDefaultRoleId);
              toast.success(t('roles.defaultRole.saved'));
            }}
            disabled={defaultRole.isLoading || defaultRole.isSaving || draftDefaultRoleId === defaultRole.currentRoleId}
          >
            {t('common:actions.save')}
          </Button>
        </div>
      </div>
    </div>
  );
}
