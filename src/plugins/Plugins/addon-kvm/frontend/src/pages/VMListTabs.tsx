// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';

export type ListTab = 'machines' | 'profiles' | 'conversion';

const VIEW_IDS: readonly ListTab[] = ['machines', 'profiles', 'conversion'];

function TabLabel({ id }: { id: ListTab }) {
  const { t } = useTranslation('vm');
  switch (id) {
    case 'machines':
      return t('list.title');
    case 'profiles':
      return t('displayProfiles.tab');
    default:
      return t('import.tab');
  }
}

export function VMListTabs({ tab, onChange }: { tab: ListTab; onChange: (tab: ListTab) => void }) {
  return (
    <div className="mb-4 flex items-center gap-1 border-b border-border">
      {VIEW_IDS.map((id) => (
        <Button
          key={id}
          variant="ghost"
          size="sm"
          aria-pressed={tab === id}
          className={
            tab === id
              ? 'rounded-none border-b-2 border-primary text-foreground'
              : 'rounded-none border-b-2 border-transparent text-muted-foreground'
          }
          onClick={() => onChange(id)}
        >
          <TabLabel id={id} />
        </Button>
      ))}
    </div>
  );
}
