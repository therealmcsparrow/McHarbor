// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// One failing widget should cost you one widget.
//
// Without a boundary, a widget that throws during render takes the whole
// grid with it — the dashboard goes blank and the user's only signal is an
// empty page. Widgets read from a dozen different endpoints, several of
// which are optional features that may not be configured, so "a widget
// throws" is a normal Tuesday rather than an exceptional case.
//
// The boundary keeps the failure inside the widget's own tile, names which
// widget failed, and offers a retry that remounts just that subtree.

import { Component, type ErrorInfo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';

type Props = {
  /** Widget label, for the message and for the console entry. */
  title: string;
  children: ReactNode;
};

type State = { error: Error | null };

class WidgetErrorBoundaryInner extends Component<
  Props & { t: (key: string, options?: Record<string, unknown>) => string },
  State
> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Named, so the console tells you which tile to go and look at.
    console.error(`[widget:${this.props.title}] render failed`, error, info);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    const { t, title } = this.props;
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-4 py-3 text-center">
        <IconAlertTriangle className="size-5 text-warning" aria-hidden="true" />
        <p className="text-sm font-medium text-foreground">{t('widget.failed', { title })}</p>
        <p className="line-clamp-2 text-xs text-muted-foreground">{error.message}</p>
        <Button variant="outline" size="sm" onClick={() => this.setState({ error: null })}>
          <IconRefresh className="size-3.5" />
          {t('widget.retry')}
        </Button>
      </div>
    );
  }
}

export function WidgetErrorBoundary({ title, children }: Props) {
  const { t } = useTranslation('dashboard');
  // Keying on the title means a widget replaced in place gets a fresh
  // boundary rather than inheriting the previous one's error.
  return (
    <WidgetErrorBoundaryInner key={title} title={title} t={t}>
      {children}
    </WidgetErrorBoundaryInner>
  );
}
