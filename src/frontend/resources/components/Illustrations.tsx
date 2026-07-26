// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cn } from '@resources/utils/cn';

type IllustrationProps = {
  className?: string;
};

export function EmptyContainersIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <rect x="20" y="30" width="80" height="50" rx="6" stroke="currentColor" strokeWidth="2" />
      <rect x="20" y="30" width="80" height="10" rx="6" stroke="currentColor" strokeWidth="2" fill="currentColor" fillOpacity="0.05" />
      <line x1="30" y1="55" x2="80" y2="55" stroke="currentColor" strokeWidth="1.5" strokeDasharray="3 3" />
      <circle cx="30" cy="55" r="2" fill="currentColor" />
      <circle cx="50" cy="55" r="2" fill="currentColor" />
      <circle cx="70" cy="55" r="2" fill="currentColor" />
      <rect x="45" y="80" width="30" height="6" rx="1" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

export function EmptyImagesIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <rect x="25" y="25" width="70" height="70" rx="4" stroke="currentColor" strokeWidth="2" />
      <path d="M25 45 L95 45" stroke="currentColor" strokeWidth="1.5" />
      <path d="M25 65 L95 65" stroke="currentColor" strokeWidth="1.5" />
      <path d="M25 85 L95 85" stroke="currentColor" strokeWidth="1.5" />
      <rect x="40" y="35" width="40" height="3" rx="1" fill="currentColor" fillOpacity="0.3" />
      <rect x="40" y="55" width="40" height="3" rx="1" fill="currentColor" fillOpacity="0.3" />
      <rect x="40" y="75" width="40" height="3" rx="1" fill="currentColor" fillOpacity="0.3" />
    </svg>
  );
}

export function EmptyNetworkIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <circle cx="60" cy="35" r="8" stroke="currentColor" strokeWidth="2" />
      <circle cx="35" cy="75" r="8" stroke="currentColor" strokeWidth="2" />
      <circle cx="85" cy="75" r="8" stroke="currentColor" strokeWidth="2" />
      <line x1="60" y1="43" x2="35" y2="67" stroke="currentColor" strokeWidth="1.5" />
      <line x1="60" y1="43" x2="85" y2="67" stroke="currentColor" strokeWidth="1.5" />
      <line x1="43" y1="75" x2="77" y2="75" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

export function EmptySearchIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <circle cx="50" cy="50" r="20" stroke="currentColor" strokeWidth="2" />
      <line x1="65" y1="65" x2="80" y2="80" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
      <line x1="42" y1="48" x2="58" y2="48" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

export function EmptyStackIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <rect x="30" y="25" width="60" height="15" rx="2" stroke="currentColor" strokeWidth="2" />
      <rect x="30" y="50" width="60" height="15" rx="2" stroke="currentColor" strokeWidth="2" />
      <rect x="30" y="75" width="60" height="15" rx="2" stroke="currentColor" strokeWidth="2" />
      <circle cx="38" cy="32" r="2" fill="currentColor" />
      <circle cx="38" cy="57" r="2" fill="currentColor" />
      <circle cx="38" cy="82" r="2" fill="currentColor" />
    </svg>
  );
}

export function EmptyEnvironmentIllustration({ className }: IllustrationProps) {
  return (
    <svg
      className={cn('text-muted-foreground/30', className)}
      width="120"
      height="120"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
    >
      <rect x="30" y="20" width="60" height="80" rx="4" stroke="currentColor" strokeWidth="2" />
      <line x1="30" y1="35" x2="90" y2="35" stroke="currentColor" strokeWidth="1.5" />
      <circle cx="80" cy="27" r="2" fill="currentColor" fillOpacity="0.4" />
      <line x1="40" y1="50" x2="80" y2="50" stroke="currentColor" strokeWidth="1" strokeDasharray="2 2" />
      <line x1="40" y1="65" x2="80" y2="65" stroke="currentColor" strokeWidth="1" strokeDasharray="2 2" />
      <line x1="40" y1="80" x2="80" y2="80" stroke="currentColor" strokeWidth="1" strokeDasharray="2 2" />
    </svg>
  );
}

export const ILLUSTRATIONS = {
  containers: EmptyContainersIllustration,
  images: EmptyImagesIllustration,
  networks: EmptyNetworkIllustration,
  search: EmptySearchIllustration,
  stacks: EmptyStackIllustration,
  environments: EmptyEnvironmentIllustration,
} as const;
