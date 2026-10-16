// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { usePolicyConflicts, type ConflictWarning } from '../../hooks/useGovernance';

type PolicyConflictWarningsProps = {
  subject: 'role' | 'policy';
};

const KIND_KEYS: Record<ConflictWarning['kind'], string> = {
  wildcard_grant: 'governance.conflicts.wildcard_grant',
  category_wildcard: 'governance.conflicts.category_wildcard',
  policy_shadowed: 'governance.conflicts.policy_shadowed',
  policy_tie: 'governance.conflicts.policy_tie',
};

export function PolicyConflictWarnings({ subject }: PolicyConflictWarningsProps) {
  const { t } = useTranslation('security');
  const allowed = usePermission([PERM.ROLES_VIEW, PERM.RBAC_POLICIES_VIEW]);
  const conflicts = usePolicyConflicts(allowed);
  const items = (conflicts.data ?? []).filter((w) => w.subjectType === subject);
  if (items.length === 0) return null;
  const warnings = items.filter((w) => w.severity === 'warning');

  return (
    <Alert
      variant={warnings.length > 0 ? 'warning' : 'info'}
      title={t(subject === 'role' ? 'governance.conflicts.rolesTitle' : 'governance.conflicts.policiesTitle', {
        count: items.length,
      })}
    >
      <ul className="mt-1 list-disc space-y-0.5 pl-4 text-xs">
        {items.map((w) => (
          <li key={`${w.kind}-${w.subjectId}-${w.relatedId ?? ''}`}>
            {t(KIND_KEYS[w.kind], {
              name: w.subjectName,
              related: w.relatedName ?? '',
              permissions: w.permissions.join(', '),
            })}
          </li>
        ))}
      </ul>
    </Alert>
  );
}
