// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Section, Field, FieldGroup } from './Form';
import { Input } from './Input';
import { NumberInput } from './NumberInput';

export default {
  title: 'Primitives / Form',
};

export const SectionShowcase = () => (
  <div className="max-w-xl space-y-3 p-8">
    <Section title="Section title">
      <p className="text-sm text-muted-foreground">Section body content goes here.</p>
    </Section>
    <Section title="Cyan section" help="Optional help caption">
      <p className="text-sm text-muted-foreground">Body</p>
    </Section>
    <Section title="Purple section">
      <span className="text-xs text-muted-foreground">body</span>
    </Section>
    <Section title="Amber section">
      <span className="text-xs text-muted-foreground">body</span>
    </Section>
    <Section title="Rose section">
      <span className="text-xs text-muted-foreground">body</span>
    </Section>
    <Section title="Green section">
      <span className="text-xs text-muted-foreground">body</span>
    </Section>
  </div>
);

export const FieldShowcase = () => (
  <div className="max-w-md space-y-4 p-8">
    <Field label="Container name" description="A short, unique identifier.">
      <Input placeholder="nginx-proxy" />
    </Field>
    <Field label="Auto-refresh interval" description="Seconds between status refreshes.">
      <NumberInput value={10} min={1} onChange={() => undefined} />
    </Field>
    <Field label="Disabled field" description="Read-only in this context.">
      <Input disabled value="—" />
    </Field>
  </div>
);

export const FieldGroupShowcase = () => (
  <div className="max-w-md p-8">
    <FieldGroup label="Notifications" description="Choose which events trigger alerts.">
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" defaultChecked />
        Container started
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" defaultChecked />
        Container stopped
      </label>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" />
        Image pull failures
      </label>
    </FieldGroup>
  </div>
);
