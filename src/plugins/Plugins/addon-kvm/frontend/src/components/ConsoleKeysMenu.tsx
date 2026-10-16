// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconKeyboard } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Popover, PopoverContent, PopoverTrigger } from '@resources/components/ui/Popover';
import { VM_PERM } from '../permissions';
import {
  SPICE_KEY_COMBOS,
  SPICE_VT_COMBOS,
  comboCaption,
  comboStrokes,
  type KeyStroke,
  type SpiceKeyCombo,
} from '../spice-keys';
import { ConsoleTypeText } from './ConsoleTypeText';

/**
 * Chords the browser or the host OS would take for itself (Ctrl+Alt+Del,
 * Alt+Tab, the Windows key, Esc), the same for every graphical console: each
 * one puts the strokes on its own wire.
 */
export function ConsoleKeysMenu({ disabled, onSend }: { disabled: boolean; onSend: (strokes: KeyStroke[]) => void }) {
  const { t } = useTranslation('vm');
  const label = (combo: SpiceKeyCombo) => {
    if (combo.named === 'windows') return t('console.keys.windows');
    if (combo.named === 'printScreen') return t('console.keys.printScreen');
    return comboCaption(combo.keys);
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm" variant="outline" permission={VM_PERM.CONSOLE} disabled={disabled}>
          <IconKeyboard className="mr-2 h-4 w-4" aria-hidden="true" />
          {t('console.keys.menu')}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 max-w-[calc(100vw-2rem)] space-y-3">
        <div className="flex flex-wrap gap-2">
          {SPICE_KEY_COMBOS.map((combo) => (
            <Button key={combo.id} size="sm" variant="outline" onClick={() => onSend(comboStrokes(combo.keys))}>
              {label(combo)}
            </Button>
          ))}
        </div>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">{t('console.keys.vtSwitch')}</p>
          <div className="grid grid-cols-6 gap-1">
            {SPICE_VT_COMBOS.map((combo) => (
              <Button
                key={combo.id}
                size="sm"
                variant="outline"
                aria-label={comboCaption(combo.keys)}
                title={comboCaption(combo.keys)}
                onClick={() => onSend(comboStrokes(combo.keys))}
              >
                {combo.keys[combo.keys.length - 1]}
              </Button>
            ))}
          </div>
        </div>
        <ConsoleTypeText onSend={onSend} />
      </PopoverContent>
    </Popover>
  );
}
