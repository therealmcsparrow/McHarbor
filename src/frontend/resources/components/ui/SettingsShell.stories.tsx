// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { SettingsShell } from './SettingsShell';
import { Section, Field } from './Form';
import { Input } from './Input';
import { Button } from './Button';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / SettingsShell',
};

export const WithFooter = () => (
  <div className="h-[480px] border border-border">
    <SettingsShell
      title="General"
      description="Site-wide preferences for this McHarbor instance."
      footer={{
        onSave: () => undefined,
        onCancel: () => undefined,
        canSave: true,
        saveLabel: 'Save changes',
      }}
      headerActions={<Button size="sm">Restore defaults</Button>}
    >
      <Stack gap={4}>
        <Section title="Site">
          <Field label="Site name" description="Shown in the page title and emails.">
            <Input defaultValue="McHarbor" />
          </Field>
          <Field label="Support URL" description="Linked from the help menu.">
            <Input defaultValue="https://docs.mcharbor.app" />
          </Field>
        </Section>
        <Section title="Telemetry">
          <Field label="Anonymous usage stats" description="Helps us prioritise fixes.">
            <Input placeholder="Disabled" />
          </Field>
        </Section>
      </Stack>
    </SettingsShell>
  </div>
);

export const NoFooter = () => (
  <Box className="h-[360px] border border-border">
    <SettingsShell title="About" description="Version info and credits.">
      <Section title="Build">
        <p className="text-sm text-muted-foreground">
          Use this layout for read-only settings tabs that don't need a save bar.
        </p>
      </Section>
    </SettingsShell>
  </Box>
);
