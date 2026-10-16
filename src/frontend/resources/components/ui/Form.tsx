// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, type ReactNode } from 'react';
import { IconHelpCircle } from '@tabler/icons-react';
import { Box, Flex } from './Layout';
import { Caption, Text as UiText } from './Typography';
import { cn } from '@resources/utils/cn';

type SectionProps = {
  icon?: ReactNode;
  title?: string;
  help?: string;
  children: ReactNode;
  className?: string;
};

/**
 * A titled group of form fields.
 *
 * The coloured 3px left border this used to draw is gone. Two reasons:
 *
 *   1. The palette was raw Tailwind (`border-l-blue-500/60`,
 *      `-cyan-500`, `-amber-500`, …), which the project's React
 *      standards forbid — hardcoded colours do not follow the OKLCH
 *      theme tokens and did not restyle with the theme.
 *   2. The colours carried no meaning. Blue, cyan, amber and purple were
 *      assigned per call site in source order, so a reader learned
 *      nothing from them; on a form with four sections it read as
 *      decoration competing with the status colours that *do* mean
 *      something.
 *
 * The neutral card border stays, which is what actually separates one
 * section from the next.
 */
function Section({ icon, title, help, children, className }: SectionProps) {
  return (
    <Box className={cn('rounded-md border border-border p-3', className)}>
      {(title || icon) && (
        <Flex align="center" gap={2} className="mb-2">
          {icon && (
            <Box className="flex size-5 shrink-0 items-center justify-center rounded-md bg-muted/60 text-muted-foreground">
              {icon}
            </Box>
          )}
          {title && (
            <UiText weight="medium" className="text-sm text-foreground">
              {title}
            </UiText>
          )}
          {help && <Caption className="ml-auto">{help}</Caption>}
        </Flex>
      )}
      {children}
    </Box>
  );
}

/**
 * FieldLabel - the standard label rendered above a form control.
 *
 * `htmlFor` is forwarded to the underlying <label>. When wrapped by
 * `<Field>`, the parent auto-generates an id and wires the htmlFor so you
 * don't have to manage it manually.
 */
function FieldLabel({ children, className, htmlFor }: { children: ReactNode; className?: string; htmlFor?: string }) {
  return (
    <label htmlFor={htmlFor} className={cn('mb-1.5 block text-xs font-medium text-muted-foreground', className)}>
      {children}
    </label>
  );
}

function FieldHelp({ children, className }: { children?: ReactNode; className?: string }) {
  if (!children) return null;
  return (
    <p className={cn('mt-1 flex items-start gap-1 text-[11px] text-muted-foreground', className)}>
      <IconHelpCircle className="mt-0.5 size-3 shrink-0" aria-hidden="true" />
      {children}
    </p>
  );
}

// Field - A consistent form-field primitive that pairs a label, the
// input slot (children), and an optional description / error message.
// Use this as the canonical wrapper inside <Section> blocks (and on
// its own) so settings forms share the same vertical rhythm.
//
// Layout:
//   <Field label="..." description="..." error="..." required>
//     <Input />      <- children
//   </Field>
//
// Aria wiring: when `label` or `description`/`error` are provided,
// Field auto-generates an id and assigns `htmlFor` on the label plus
// matching `aria-describedby` on the input wrapper. Pass `id` to
// override the generated id (callers that need a stable id for the
// surrounding control should set this). The id is exposed via
// the `id` prop so inputs can spread it (e.g. <Input id={fieldId} />).
type FieldProps = {
  label?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  children: ReactNode;
  className?: string;
  /** Override the auto-generated id used for label / aria wiring. */
  id?: string;
  /** Optional explicit id forwarded to the description span (auto-derived if omitted). */
  descriptionId?: string;
  /** Optional explicit id forwarded to the error span (auto-derived if omitted). */
  errorId?: string;
};

function Field({
  label,
  description,
  error,
  required,
  children,
  className,
  id,
  descriptionId: descriptionIdOverride,
  errorId: errorIdOverride,
}: FieldProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const descriptionId = descriptionIdOverride ?? `${fieldId}-description`;
  const errorId = errorIdOverride ?? `${fieldId}-error`;

  // Build the aria-describedby list: include the description and/or error
  // ids only when they're actually rendered so we don't point screen
  // readers at empty nodes.
  const describedBy =
    [description ? descriptionId : null, error ? errorId : null].filter(Boolean).join(' ') || undefined;

  return (
    <Box className={cn('space-y-1.5', className)}>
      {label && (
        <label htmlFor={fieldId} className="mb-1.5 block text-xs font-medium text-muted-foreground">
          {label}
          {required && <span className="ml-0.5 text-destructive">*</span>}
        </label>
      )}
      <FieldSlot describedBy={describedBy}>{children}</FieldSlot>
      {description && (
        <p id={descriptionId} className="text-[11px] text-muted-foreground">
          {description}
        </p>
      )}
      {error && (
        <p id={errorId} className="text-[11px] text-destructive">
          {error}
        </p>
      )}
    </Box>
  );
}

/**
 * FieldSlot - the children wrapper inside Field. It applies
 * `aria-describedby` so screen readers pick up the description/error
 * paragraphs. Callers that need a stable id on the input should set
 * `id` on Field and forward it manually (e.g. <Input id={id} />).
 */
function FieldSlot({ describedBy, children }: { describedBy: string | undefined; children: ReactNode }) {
  return (
    <div aria-describedby={describedBy} className="min-w-0">
      {children}
    </div>
  );
}

// FieldGroup - A fieldset/legend wrapper for grouped controls such as
// radio rows or checkbox lists. Pairs a legend (label + optional
// required marker) with the children stack, plus an optional
// description and error paragraph below. The fieldset is intentionally
// not given a `disabled` attribute - disable children individually
// when needed, so the fieldset itself does not blanket-disable them.
//
// Layout:
//   <FieldGroup label="..." description="..." error="..." required>
//     <RadioRow />  <- children
//     <RadioRow />
//   </FieldGroup>
//
// Aria wiring: when `label` or `description`/`error` are provided,
// FieldGroup auto-generates ids and assigns `aria-describedby` on the
// fieldset. Pass `id` to override the generated id (callers that need
// a stable id for the surrounding group should set this).
type FieldGroupProps = {
  label?: ReactNode;
  description?: ReactNode;
  error?: ReactNode;
  required?: boolean;
  children: ReactNode;
  className?: string;
  /** Optional explicit id forwarded to the fieldset (auto-derived if omitted). */
  id?: string;
  /** Optional explicit id forwarded to the description span (auto-derived if omitted). */
  descriptionId?: string;
  /** Optional explicit id forwarded to the error span (auto-derived if omitted). */
  errorId?: string;
};

function FieldGroup({
  label,
  description,
  error,
  required,
  children,
  className,
  id,
  descriptionId: descriptionIdOverride,
  errorId: errorIdOverride,
}: FieldGroupProps) {
  const generatedId = useId();
  const fieldsetId = id ?? generatedId;
  const descriptionId = descriptionIdOverride ?? `${fieldsetId}-description`;
  const errorId = errorIdOverride ?? `${fieldsetId}-error`;

  // Build the aria-describedby list: include the description and/or error
  // ids only when they're actually rendered so we don't point screen
  // readers at empty nodes.
  const describedBy =
    [description ? descriptionId : null, error ? errorId : null].filter(Boolean).join(' ') || undefined;

  return (
    <fieldset className={cn('space-y-2 rounded-md border border-border p-3', className)} aria-describedby={describedBy}>
      {label && (
        <legend className="px-1 text-xs font-medium text-muted-foreground">
          {label}
          {required && <span className="ml-0.5 text-destructive">*</span>}
        </legend>
      )}
      <div className="space-y-2">{children}</div>
      {description && (
        <p id={descriptionId} className="text-[11px] text-muted-foreground">
          {description}
        </p>
      )}
      {error && (
        <p id={errorId} className="text-[11px] text-destructive">
          {error}
        </p>
      )}
    </fieldset>
  );
}

export { Section, Field, FieldGroup, FieldLabel, FieldHelp };
export type { SectionProps, FieldProps, FieldGroupProps };
