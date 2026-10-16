// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Deciding which field a secret can be inserted into, and putting it there.
 *
 * Split out from the provider so the two hard parts — "is this a field a
 * reference belongs in?" and "how do you write to a React-controlled input
 * without React undoing it?" — are testable and stated once.
 */

/** The input types a `${secret:NAME}` reference makes sense in. */
const INSERTABLE_INPUT_TYPES = new Set([
  'text',
  'number',
  'password',
  'search',
  'url',
  'email',
  'tel',
  // An input with no type attribute is a text input.
  '',
]);

export type InsertTarget = HTMLInputElement | HTMLTextAreaElement;

/**
 * Reports whether an element can take an inserted secret reference.
 *
 * Deliberately narrow. A checkbox, a date picker or a file input cannot hold
 * `${secret:NAME}`, and a readonly or disabled field is one the user is not
 * editing — intercepting the context menu on any of those would take away the
 * browser's own menu for nothing.
 */
export function asInsertTarget(node: EventTarget | null): InsertTarget | null {
  if (!(node instanceof HTMLElement)) return null;

  if (node instanceof HTMLTextAreaElement) {
    return node.readOnly || node.disabled ? null : node;
  }
  if (node instanceof HTMLInputElement) {
    if (node.readOnly || node.disabled) return null;
    return INSERTABLE_INPUT_TYPES.has(node.type.toLowerCase()) ? node : null;
  }
  return null;
}

/**
 * Writes `text` into the field at the caret, replacing any selection.
 *
 * The value is set through the **native** property setter rather than by
 * assignment. React tracks an input's last-rendered value on the DOM node
 * itself; a plain `el.value = x` updates the DOM but leaves that tracker
 * untouched, so React's synthetic `onChange` never fires, and the next render
 * puts the old value straight back. Going through the prototype's setter and
 * then dispatching a bubbling `input` event is what makes a controlled field
 * actually accept the write.
 *
 * A `number` input cannot hold `${secret:NAME}` — the browser rejects the
 * assignment and the field goes empty — so it is switched to `text` first.
 * That is the honest trade: the field stops enforcing numeric input for as
 * long as it holds a reference, which is exactly as long as the value is not
 * a number.
 */
export function insertSecretReference(target: InsertTarget, text: string): void {
  if (target instanceof HTMLInputElement && target.type.toLowerCase() === 'number') {
    target.type = 'text';
  }

  const start = target.selectionStart ?? target.value.length;
  const end = target.selectionEnd ?? target.value.length;
  const next = target.value.slice(0, start) + text + target.value.slice(end);

  const prototype = target instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const descriptor = Object.getOwnPropertyDescriptor(prototype, 'value');

  if (descriptor?.set) {
    descriptor.set.call(target, next);
  } else {
    // No descriptor is not a case any supported browser reaches; falling back
    // to assignment keeps an uncontrolled field working rather than throwing.
    target.value = next;
  }

  target.dispatchEvent(new Event('input', { bubbles: true }));

  const caret = start + text.length;
  target.setSelectionRange?.(caret, caret);
  target.focus();
}
