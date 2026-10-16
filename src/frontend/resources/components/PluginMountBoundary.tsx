// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Component, type ErrorInfo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState, type ErrorStateVariant } from '@resources/components/ui/ErrorState';

type Props = {
  children: ReactNode;
  variant?: ErrorStateVariant;
  className?: string;
};

type InnerProps = Props & { title: string };

type State = { error: Error | null; attempt: number };

/** A render error is a React-boundary event; a lazy chunk that rejected stays rejected until a reload. */
class PluginMountBoundaryInner extends Component<InnerProps, State> {
  state: State = { error: null, attempt: 0 };

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[mount] subtree failed to render', error, info);
  }

  retry = () => this.setState((s) => ({ error: null, attempt: s.attempt + 1 }));

  render() {
    const { error, attempt } = this.state;
    if (error) {
      return (
        <ErrorState
          variant={this.props.variant ?? 'card'}
          className={this.props.className}
          title={this.props.title}
          error={error}
          onRetry={this.retry}
        />
      );
    }
    return (
      <div key={attempt} className="contents">
        {this.props.children}
      </div>
    );
  }
}

export function PluginMountBoundary({ children, variant, className }: Props) {
  const { t } = useTranslation('common');
  return (
    <PluginMountBoundaryInner title={t('pluginMount.failed', { ns: 'common' })} variant={variant} className={className}>
      {children}
    </PluginMountBoundaryInner>
  );
}
