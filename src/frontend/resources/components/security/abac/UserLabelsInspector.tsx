// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTags } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Badge } from '@resources/components/ui/Badge';
import { useUserLabels } from '@resources/hooks/usePolicies';

type UserLabelsInspectorProps = {
  /** Merge the fetched labels into the tester's attribute bundle. */
  onApply: (labels: Record<string, string>) => void;
};

/**
 * Look up a real user's labels and drop them into the test bundle.
 *
 * `useUserLabels` was written for exactly this — its own comment says it is
 * "used by ABAC authors to inspect what they can match against user.labels in
 * policy expressions" — but nothing rendered it, so the tester could only ever
 * be run against hand-typed labels. An expression that passes against invented
 * labels and fails against the real ones is the failure this closes.
 */
export function UserLabelsInspector({ onApply }: UserLabelsInspectorProps) {
  const { t } = useTranslation('abac');
  const [userId, setUserId] = useState('');
  const [lookup, setLookup] = useState('');
  const { data: labels, isFetching } = useUserLabels(lookup);

  const entries = Object.entries(labels ?? {});

  return (
    <div className="space-y-2 rounded-md border border-border bg-background/40 p-3">
      <p className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <IconTags className="size-3.5" aria-hidden="true" />
        {t('tester.userLabelsTitle')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Input
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          placeholder={t('tester.userIdPlaceholder')}
          className="min-w-0 flex-1"
        />
        <Button
          variant="outline"
          size="sm"
          onClick={() => setLookup(userId.trim())}
          disabled={!userId.trim() || isFetching}
        >
          {t('tester.lookup')}
        </Button>
      </div>

      {lookup &&
        !isFetching &&
        (entries.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('tester.noLabels')}</p>
        ) : (
          <>
            <div className="flex flex-wrap gap-1">
              {entries.map(([key, value]) => (
                <Badge key={key} variant="secondary">
                  {key}={value}
                </Badge>
              ))}
            </div>
            <Button variant="ghost" size="sm" onClick={() => onApply(labels ?? {})}>
              {t('tester.applyLabels')}
            </Button>
          </>
        ))}
    </div>
  );
}
