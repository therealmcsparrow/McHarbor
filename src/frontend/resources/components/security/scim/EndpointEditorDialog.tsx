// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { Endpoint, EndpointInput } from '@resources/hooks/useScimEndpoints';
import { PERM } from '@core/auth/permissionKeys';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial: Endpoint | null;
  onSubmit: (input: EndpointInput) => void;
  isSubmitting: boolean;
};

const DEFAULT_SCHEDULE = '*/15 * * * *';

// EndpointEditorDialog collects the fields for both create + edit.
// The bearer-token field is a write-only input — on edit, we leave
// it blank by default and only overwrite the server's token when
// the user actively supplies a new value. The TokenRevealDialog
// handles the one-time plaintext display after a successful create.
export function EndpointEditorDialog({ open: isOpen, onOpenChange, initial, onSubmit, isSubmitting }: Props) {
  const { t } = useTranslation('scim');
  const blank = useMemo<EndpointInput>(
    () => ({
      name: '',
      baseUrl: '',
      authType: 'bearer',
      bearerToken: '',
      username: '',
      pollSchedule: DEFAULT_SCHEDULE,
      enabled: true,
    }),
    [],
  );
  const formFor = (source: typeof initial): EndpointInput =>
    source
      ? {
          name: source.name,
          baseUrl: source.baseUrl,
          authType: source.authType,
          bearerToken: '', // Never pre-fill tokens from the server.
          username: source.username ?? '',
          pollSchedule: source.pollSchedule,
          enabled: source.enabled,
        }
      : blank;
  const [form, setForm] = useState<EndpointInput>(() => formFor(initial));
  const [synced, setSynced] = useState({ initial, isOpen });

  if (synced.initial !== initial || synced.isOpen !== isOpen) {
    setSynced({ initial, isOpen });
    setForm(formFor(initial));
  }

  const handleSubmit = () => {
    const payload: EndpointInput = {
      ...form,
      bearerToken: form.bearerToken?.trim() || undefined,
    };
    onSubmit(payload);
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{initial ? t('editor.titleEdit') : t('editor.titleCreate')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label htmlFor="scim-name">{t('editor.name')}</Label>
              <Input
                id="scim-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder={t('editor.namePlaceholder')}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="scim-auth">{t('editor.authType')}</Label>
              <Select
                value={form.authType}
                onChange={(v) => setForm({ ...form, authType: v })}
                options={[
                  { value: 'bearer', label: t('editor.authBearer') },
                  { value: 'basic', label: t('editor.authBasic') },
                  { value: 'none', label: t('editor.authNone') },
                ]}
                ariaLabel={t('editor.authType')}
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="scim-url">{t('editor.baseUrl')}</Label>
            <Input
              id="scim-url"
              value={form.baseUrl}
              onChange={(e) => setForm({ ...form, baseUrl: e.target.value })}
              placeholder={t('editor.baseUrlPlaceholder')}
            />
          </div>

          {form.authType !== 'none' && (
            <>
              <div className="space-y-1">
                <Label htmlFor="scim-token">
                  {t('editor.token')}
                  {initial && form.authType === 'bearer' && (
                    <span className="ml-2 text-xs text-muted-foreground">
                      {initial.hasToken ? t('editor.tokenStored') : t('editor.tokenMissing')}
                    </span>
                  )}
                </Label>
                <Input
                  id="scim-token"
                  type="password"
                  value={form.bearerToken ?? ''}
                  onChange={(e) => setForm({ ...form, bearerToken: e.target.value })}
                  placeholder={initial ? t('editor.tokenKeep') : t('editor.tokenAutoGenerate')}
                />
                <p className="text-xs text-muted-foreground">{t('editor.tokenHint')}</p>
              </div>
              {form.authType === 'basic' && (
                <div className="space-y-1">
                  <Label htmlFor="scim-user">{t('editor.username')}</Label>
                  <Input
                    id="scim-user"
                    value={form.username ?? ''}
                    onChange={(e) => setForm({ ...form, username: e.target.value })}
                    placeholder={t('editor.usernamePlaceholder')}
                  />
                </div>
              )}
            </>
          )}

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label htmlFor="scim-schedule">{t('editor.pollSchedule')}</Label>
              <Input
                id="scim-schedule"
                value={form.pollSchedule}
                onChange={(e) => setForm({ ...form, pollSchedule: e.target.value })}
                placeholder={DEFAULT_SCHEDULE}
              />
              <p className="text-xs text-muted-foreground">{t('editor.pollScheduleHint')}</p>
            </div>
            <div className="flex items-end gap-2 pb-1">
              <Switch
                id="scim-enabled"
                checked={form.enabled}
                onCheckedChange={(checked) => setForm({ ...form, enabled: checked })}
              />
              <Label htmlFor="scim-enabled" className="cursor-pointer">
                {t('editor.enabled')}
              </Label>
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('editor.cancel')}
          </Button>
          <Button
            permission={PERM.SCIM_MANAGE}
            onClick={handleSubmit}
            disabled={!form.name || !form.baseUrl || isSubmitting}
          >
            {isSubmitting ? t('editor.saving') : t('editor.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
