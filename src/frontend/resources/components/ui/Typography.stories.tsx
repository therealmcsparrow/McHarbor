// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Title, Heading, Subheading, Text, Caption, Code } from './Typography';

export default {
  title: 'Primitives / Typography',
};

export const HeadingScale = () => (
  <div className="max-w-2xl space-y-3 p-8">
    <Title size="2xl">Title 2xl — page header</Title>
    <Title size="xl">Title xl — page header</Title>
    <Title size="lg">Title lg — page header</Title>
    <Heading>Heading (h2)</Heading>
    <Subheading>Subheading (h3)</Subheading>
    <Text>Body text — default tone, default size.</Text>
    <Text tone="muted" size="sm">
      Muted body — descriptions, helper text.
    </Text>
    <Caption>caption — timestamps, labels</Caption>
  </div>
);

export const Tones = () => (
  <div className="max-w-xl space-y-2 p-8">
    <Text tone="default">Default — primary text</Text>
    <Text tone="muted">Muted — secondary text</Text>
    <Text tone="subtle">Subtle — tertiary text</Text>
    <Text tone="primary">Primary — brand color</Text>
    <Text tone="success">Success — green</Text>
    <Text tone="warning">Warning — amber</Text>
    <Text tone="destructive">Destructive — red</Text>
  </div>
);

export const CodeBlocks = () => (
  <div className="max-w-xl space-y-3 p-8">
    <Text>
      Inline <Code>container.id</Code> reference.
    </Text>
    <Code>mcharbor-mcharbor-1</Code>
  </div>
);
