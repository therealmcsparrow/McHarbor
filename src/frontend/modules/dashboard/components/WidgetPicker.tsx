// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { cn } from '@resources/utils/cn';
import {
  WIDGET_CATEGORIES,
  useWidgetDefinitions,
  type WidgetTypeId,
} from '../widgets/registry';
import { useDashboardLayoutStore } from '../stores/dashboard-layout';

type WidgetPickerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function WidgetPicker({ open, onOpenChange }: WidgetPickerProps) {
  const { t } = useTranslation('dashboard');
  const { widgets, addWidget, removeWidget } = useDashboardLayoutStore();
  const allDefinitions = useWidgetDefinitions();
  const activeTypeIds = new Set(widgets.map((w) => w.typeId));

  function toggle(typeId: WidgetTypeId) {
    if (activeTypeIds.has(typeId)) {
      const instance = widgets.find((w) => w.typeId === typeId);
      if (instance) removeWidget(instance.id);
    } else {
      addWidget(typeId);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-0">
        <DialogHeader>
          <DialogTitle>{t('addWidgets')}</DialogTitle>
          <DialogDescription>{t('addWidgetsDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="p-4">
          {WIDGET_CATEGORIES.map((cat) => {
            const catWidgets = allDefinitions.filter((w) => w.category === cat.id);
            if (catWidgets.length === 0) return null;
            return (
              <div key={cat.id} className="mb-5 last:mb-0">
                <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  {t(cat.labelKey)}
                </h3>
                <div className="grid grid-cols-2 gap-2">
                  {catWidgets.map((def) => {
                    const isActive = activeTypeIds.has(def.id);
                    const Icon = def.icon;
                    return (
                      <Button
                        key={def.id}
                        variant="ghost"
                        onClick={() => toggle(def.id)}
                        className={cn(
                          'flex h-auto items-start gap-3 whitespace-normal rounded-lg border p-3 text-left transition-colors',
                          isActive
                            ? 'border-primary/50 bg-primary/5'
                            : 'border-border hover:border-primary/30 hover:bg-accent/50'
                        )}
                      >
                        <div className={cn('mt-0.5 shrink-0', isActive ? 'text-primary' : 'text-muted-foreground')}>
                          <Icon className="h-5 w-5" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-medium text-foreground">{t(def.labelKey)}</p>
                          <p className="text-xs text-muted-foreground">{t(def.descriptionKey)}</p>
                        </div>
                        <div className="mt-0.5 shrink-0">
                          {isActive ? (
                            <IconCheck className="h-4 w-4 text-primary" />
                          ) : (
                            <IconPlus className="h-4 w-4 text-muted-foreground" />
                          )}
                        </div>
                      </Button>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
