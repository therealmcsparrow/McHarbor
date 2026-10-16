// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { typeDescription, typeName } from './catalog-labels';
import { ConfigureFields } from './ConfigureFields';
import { ConfigureFooter } from './ConfigureFooter';
import { useConfigureState } from './useConfigureState';
import type { GenerateRequest, ReportType, ScheduleRequest } from '../types';

// The configure step, as the host's standard dialog.
//
// Everything here has a working default from the catalog entry, so the
// fast path is still: pick a report, press Generate. The controls exist
// for the cases where the default is wrong, not to make the user answer
// four questions before they can see anything.

type Props = {
  type: ReportType | null;
  pending: boolean;
  error: boolean;
  onClose: () => void;
  onGenerate: (request: GenerateRequest) => void;
  /**
   * Called instead of `onGenerate` when the schedule section is on. A
   * schedule is a different verb on the same configuration, so it shares
   * the dialog rather than duplicating every control in a second one.
   */
  onSchedule: (request: ScheduleRequest) => void;
};

export function ConfigureDialog({ type, pending, error, onClose, onGenerate, onSchedule }: Props) {
  return (
    <Dialog
      open={type !== null}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !pending) onClose();
      }}
    >
      {/* Keyed on the type so switching reports resets every control to
          that type's own defaults, rather than carrying the previous
          report's period and sections across.

          `ConfigureForm` renders `DialogContent` itself rather than being
          rendered inside one: `DialogContent` sorts its slots by direct
          child identity (`isDialogSlot`), so a header and footer arriving
          wrapped in a component would be swept into an implicit
          `DialogBody` and scroll with the form. */}
      {type ? (
        <ConfigureForm
          key={type.key}
          type={type}
          pending={pending}
          error={error}
          onClose={onClose}
          onGenerate={onGenerate}
          onSchedule={onSchedule}
        />
      ) : null}
    </Dialog>
  );
}

function ConfigureForm({ type, pending, error, onClose, onGenerate, onSchedule }: Props & { type: ReportType }) {
  const { t } = useTranslation('reporting');
  const state = useConfigureState({ type, onGenerate, onSchedule });

  return (
    <DialogContent className="max-w-2xl">
      <DialogHeader>
        <DialogTitle>{typeName(t, type)}</DialogTitle>
        <DialogDescription>{typeDescription(t, type)}</DialogDescription>
      </DialogHeader>

      <DialogBody className="space-y-5">
        <ConfigureFields type={type} state={state} />
      </DialogBody>

      <DialogFooter className="items-center justify-between gap-3 sm:justify-between">
        <ConfigureFooter type={type} state={state} pending={pending} error={error} onClose={onClose} />
      </DialogFooter>
    </DialogContent>
  );
}
