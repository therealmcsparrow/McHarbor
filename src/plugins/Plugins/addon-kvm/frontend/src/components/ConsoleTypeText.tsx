// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import { textStrokes, type KeyStroke } from '../spice-keys';

/**
 * Types a string into the guest as key presses. Neither console has a clipboard
 * without an agent in the guest, so this is how a password reaches a login prompt.
 */
export function ConsoleTypeText({ onSend }: { onSend: (strokes: KeyStroke[]) => void }) {
  const { t } = useTranslation('vm');
  const id = useId();
  const [text, setText] = useState('');
  const [skipped, setSkipped] = useState(0);

  const submit = () => {
    const typed = textStrokes(text);
    if (typed.strokes.length > 0) onSend(typed.strokes);
    setSkipped(typed.skipped);
    if (typed.skipped === 0) setText('');
  };

  return (
    <div className="space-y-1.5 border-t border-border pt-3">
      <Label htmlFor={id}>{t('console.keys.typeText')}</Label>
      <Textarea
        id={id}
        rows={2}
        value={text}
        onChange={(evt) => setText(evt.target.value)}
        autoComplete="off"
        spellCheck={false}
      />
      <p className="text-xs text-muted-foreground">{t('console.keys.typeTextHint')}</p>
      {skipped > 0 && (
        <output className="block text-xs text-warning">{t('console.keys.skipped', { count: skipped })}</output>
      )}
      <Button size="sm" className="w-full" disabled={text.length === 0} onClick={submit}>
        {t('console.keys.typeTextSend')}
      </Button>
    </div>
  );
}
