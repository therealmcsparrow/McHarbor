// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The TLS listener status is read by the shared useTLS hook and rendered by
// both the settings and the security surfaces, so the shape lives in core
// rather than in either module.

export type CertInfo = {
  subject: string;
  issuer: string;
  notBefore: string;
  notAfter: string;
  serialNumber: string;
  dnsNames: string[];
  fingerprint?: string;
};

export type TLSStatus = {
  enabled: boolean;
  forceHttps: boolean;
  hasCert: boolean;
  certInfo?: CertInfo;
  // certSource: 'self-signed' when the auto-generated self-signed cert
  // is in use (Phase 1 default-HTTPS), 'user-uploaded' when an admin
  // uploaded a custom cert via the UI.
  certSource?: 'self-signed' | 'user-uploaded';
  // tlsMinVersion is the floor TLS version the listeners enforce
  // (12 = TLS 1.2, 13 = TLS 1.3) — surfaced for operator visibility.
  tlsMinVersion?: number;
  // hstsMaxAge is the Strict-Transport-Security max-age in seconds.
  // 0 means HSTS is disabled (the safe default to preserve the
  // HTTP→HTTPS recovery redirect listener).
  hstsMaxAge?: number;
  // transferPort is the listener port for the agent file-transfer
  // helper (default 8788). Surfaced so operators can verify it
  // matches their published compose port mapping.
  transferPort?: number;
  hasCa?: boolean;
  caFingerprint?: string;
  caNotAfter?: string;
};
