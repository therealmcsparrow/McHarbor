// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// i18n keys used by this file:
//   settings:branding.{choose,replace,clear,previewAlt,usingDefault,tooLarge,wrongType,readFailed}

import { useId, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPhotoUp, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import { cn } from '@resources/utils/cn';
import { BRANDING_ACCEPT, BRANDING_MAX_BYTES } from '@resources/hooks/useBranding';
import { rasterizeMark } from '@resources/utils/markRaster';

type BrandingLogoFieldProps = {
  label: string;
  description: string;
  /** The current value: a `data:` URI, or '' for "use the bundled mark". */
  value: string;
  /** The bundled asset, shown greyed-out while `value` is empty. */
  fallbackSrc: string;
  /**
   * `raster` is a base64 PNG rendition of the same image, produced only when
   * `withRaster` is set. Document exporters embed raster only, and the
   * browser is the one place in the stack that already has a rasteriser.
   */
  onChange: (value: string, raster: string) => void;
  /** Produce a PNG rendition alongside the upload. Light slots only. */
  withRaster?: boolean;
  disabled?: boolean;
  permission?: string;
  /** Extra classes for the preview frame — the two marks have very
   *  different aspect ratios, so each caller sizes its own. */
  previewClassName?: string;
  /** Which ground the preview paints behind the image. */
  ground: 'light' | 'dark';
};

/**
 * The preview backdrops.
 *
 * These are fixed values rather than `bg-sidebar`, and that is the point:
 * both variants are on screen at once, so a backdrop that followed the active
 * theme would show the light slot on a dark ground half the time — the exact
 * mistake this preview exists to catch. `.dark` is a plain class on <html>, so
 * a wrapper can force dark but cannot force light back (light is `:root`),
 * which rules out doing this with tokens. They approximate the default
 * theme's two sidebar grounds.
 */
const GROUND: Record<'light' | 'dark', string> = {
  light: 'bg-[oklch(0.96_0.005_254)]',
  dark: 'bg-[oklch(0.148_0.024_254)]',
};

/**
 * One uploadable mark: preview, pick, clear.
 *
 * The file is read to a data URI in the browser rather than posted as
 * multipart, because that is what ends up in the settings row either way —
 * doing the encoding here means the preview is the exact bytes that will be
 * stored, so what an operator approves is what every visitor sees.
 *
 * Validation is mirrored from the backend rather than trusted to it: a
 * 4MB PNG should be refused before it is base64-encoded into a request, and
 * the message can name the actual limit instead of a generic 400.
 */
export function BrandingLogoField({
  label,
  description,
  value,
  fallbackSrc,
  onChange,
  disabled = false,
  permission,
  previewClassName,
  ground,
  withRaster = false,
}: BrandingLogoFieldProps) {
  const { t } = useTranslation('settings');
  const inputRef = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const [error, setError] = useState('');

  function handlePick(changeEvent: ChangeEvent<HTMLInputElement>) {
    const file = changeEvent.target.files?.[0];
    // The picker is reset unconditionally so re-choosing the same file after
    // a rejection still fires a change event.
    changeEvent.target.value = '';
    if (!file) return;
    setError('');

    if (!BRANDING_ACCEPT.split(',').includes(file.type)) {
      setError(t('branding.wrongType'));
      return;
    }
    if (file.size > BRANDING_MAX_BYTES) {
      setError(t('branding.tooLarge', { size: Math.round(BRANDING_MAX_BYTES / 1024) }));
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result;
      if (typeof result !== 'string') {
        setError(t('branding.readFailed'));
        return;
      }
      if (!withRaster) {
        onChange(result, '');
        return;
      }
      // The rendition is a convenience for document exporters, so a failure
      // to produce one resolves to '' rather than blocking the upload the
      // operator can plainly see in the preview.
      void rasterizeMark(result).then((raster) => onChange(result, raster));
    };
    reader.onerror = () => setError(t('branding.readFailed'));
    reader.readAsDataURL(file);
  }

  return (
    <SettingsField
      label={label}
      description={description}
      hint={error ? <span className="text-destructive">{error}</span> : !value ? t('branding.usingDefault') : undefined}
    >
      <div className="flex flex-wrap items-center gap-4">
        {/* See GROUND — the backdrop is pinned to this slot's variant rather
            than the active theme, so both previews tell the truth at once. */}
        <div
          className={cn(
            'flex shrink-0 items-center justify-center rounded-lg border border-border p-3',
            GROUND[ground],
            previewClassName,
          )}
        >
          <img
            src={value || fallbackSrc}
            alt={t('branding.previewAlt', { mark: label })}
            className={cn('max-h-full max-w-full object-contain', !value && 'opacity-50')}
          />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <input
            ref={inputRef}
            id={inputId}
            type="file"
            accept={BRANDING_ACCEPT}
            className="hidden"
            onChange={handlePick}
            disabled={disabled}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            permission={permission}
            disabled={disabled}
            onClick={() => inputRef.current?.click()}
          >
            <IconPhotoUp className="size-4" />
            {value ? t('branding.replace') : t('branding.choose')}
          </Button>
          {value && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              permission={permission}
              disabled={disabled}
              onClick={() => {
                setError('');
                onChange('', '');
              }}
            >
              <IconTrash className="size-4" />
              {t('branding.clear')}
            </Button>
          )}
        </div>
      </div>
    </SettingsField>
  );
}
