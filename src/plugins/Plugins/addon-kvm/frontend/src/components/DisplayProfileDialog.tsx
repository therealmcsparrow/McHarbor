// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { VM_PERM } from '../permissions';
import { useSaveDisplayProfile } from '../hooks/useDisplayProfiles';
import {
  DISPLAY_PROFILE_OPTIONS,
  NEW_PROFILE_DRAFT,
  type DisplayProfile,
  type DisplayProfileDraft,
  type DisplayProfileOption,
} from '../types-display';
import { optionHint, optionLabel, profileName, valueLabel } from './displayProfileLabels';

function draftFrom(profile: DisplayProfile | null): DisplayProfileDraft {
  if (!profile) return NEW_PROFILE_DRAFT;
  return {
    name: profile.name,
    description: profile.description ?? '',
    imageCompression: profile.imageCompression,
    jpegWanCompression: profile.jpegWanCompression,
    zlibGlzWanCompression: profile.zlibGlzWanCompression,
    streamingVideo: profile.streamingVideo,
    playbackCompression: profile.playbackCompression,
    agentMouse: profile.agentMouse,
  };
}

/** Create a custom profile (profile null) or edit one; built-ins never reach here. */
export function DisplayProfileDialog({ profile, onClose }: { profile: DisplayProfile | null; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const save = useSaveDisplayProfile();
  const [draft, setDraft] = useState<DisplayProfileDraft>(() => draftFrom(profile));
  // isPending reaches the button a render late; a second click in that window
  // must not create a second profile.
  const sending = useRef(false);
  const nameMissing = draft.name.trim() === '';

  const setOption = (key: DisplayProfileOption, value: string) => setDraft((current) => ({ ...current, [key]: value }));

  const submit = () => {
    if (sending.current || nameMissing) return;
    sending.current = true;
    save.mutate(
      { id: profile?.id, body: { ...draft, name: draft.name.trim(), description: draft.description.trim() } },
      {
        onSuccess: onClose,
        onSettled: () => {
          sending.current = false;
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && !save.isPending && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {profile
              ? t('displayProfiles.dialog.editTitle', { name: profileName(t, profile) })
              : t('displayProfiles.dialog.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('displayProfiles.dialog.nextStartNote')}</DialogDescription>
        </DialogHeader>

        <div className="grid max-h-[60vh] gap-4 overflow-y-auto sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor="display-profile-name">{t('displayProfiles.dialog.name')}</Label>
            <Input
              id="display-profile-name"
              value={draft.name}
              maxLength={80}
              disabled={save.isPending}
              aria-invalid={nameMissing}
              onChange={(evt) => setDraft((current) => ({ ...current, name: evt.target.value }))}
            />
            {nameMissing && <p className="text-xs text-destructive">{t('displayProfiles.dialog.nameRequired')}</p>}
          </div>
          <div className="space-y-1">
            <Label htmlFor="display-profile-description">{t('displayProfiles.dialog.descriptionLabel')}</Label>
            <Input
              id="display-profile-description"
              value={draft.description}
              maxLength={500}
              disabled={save.isPending}
              onChange={(evt) => setDraft((current) => ({ ...current, description: evt.target.value }))}
            />
          </div>
          {DISPLAY_PROFILE_OPTIONS.map((option) => (
            <div key={option.key} className="space-y-1">
              <Label>{optionLabel(t, option.key)}</Label>
              <Select
                value={draft[option.key]}
                ariaLabel={optionLabel(t, option.key)}
                disabled={save.isPending}
                searchable={false}
                options={option.values.map((value) => ({ value, label: valueLabel(t, value) }))}
                onChange={(value) => setOption(option.key, value)}
              />
              <p className="text-xs text-muted-foreground">{optionHint(t, option.key)}</p>
            </div>
          ))}
        </div>

        {save.isError && <Alert variant="destructive" description={save.error.message} />}

        <DialogFooter>
          <Button variant="outline" disabled={save.isPending} onClick={onClose}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={save.isPending || nameMissing} onClick={submit}>
            {t('displayProfiles.dialog.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
