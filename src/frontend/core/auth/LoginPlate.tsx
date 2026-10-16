// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * The plates, in rotation order. All three are free-licence Unsplash
 * photographs served locally — see `login-plate.CREDITS.md` next to this file
 * for provenance. Every one is brand-free: a login screen for a product
 * distributed under its own licence has no business carrying somebody else's
 * trademarks.
 */
const PLATES = [
  { file: 'login-plate-1', credit: 'Roman Chukhin' },
  { file: 'login-plate-2', credit: 'Cedric Letsch' },
  { file: 'login-plate-3', credit: 'Mick Haupt' },
] as const;

const ROTATE_MS = 30_000;

/**
 * The left half of the login page: a photographic plate pushed down into a
 * cyanotype, with the control-plane schematic plotted over it.
 *
 * It is deliberately presentational and stateless — the pre-auth page can
 * read nothing about the fleet, so nothing here is a live figure. The
 * dimension line states a property of the architecture ("zero exposed
 * sockets"), never a count, because a login screen that quotes numbers it
 * cannot have queried is lying to the person reading it.
 *
 * Hidden below `lg`: on a phone the form is the whole page.
 */
export function LoginPlate() {
  const { t } = useTranslation('auth');
  const [plate, setPlate] = useState(0);
  // `noUncheckedIndexedAccess` is on, and a variable index is unchecked — the
  // tuple's literal index is not, so this is a real narrowing, not a cast.
  const active = PLATES[plate] ?? PLATES[0];

  useEffect(() => {
    const id = window.setInterval(() => {
      setPlate((i) => (i + 1) % PLATES.length);
    }, ROTATE_MS);
    return () => window.clearInterval(id);
  }, []);

  return (
    <aside className="mc-login-plate relative hidden overflow-hidden lg:flex lg:flex-col">
      {/* All three plates are mounted and crossfaded by opacity: 43 KB of AVIF
          for the set, so a swap never waits on the network and never shows a
          gap. The fade is killed under either reduce-motion setting — see
          `.mc-login-photo` in app.css. */}
      {PLATES.map((entry, i) => (
        <picture key={entry.file}>
          <source srcSet={`/${entry.file}.avif`} type="image/avif" />
          <source srcSet={`/${entry.file}.webp`} type="image/webp" />
          <img
            src={`/${entry.file}.jpg`}
            alt=""
            width={1400}
            height={927}
            aria-hidden="true"
            className={`mc-login-photo absolute inset-0 size-full scale-[1.03] object-cover transition-opacity duration-1000 ${
              i === plate ? 'opacity-100' : 'opacity-0'
            }`}
          />
        </picture>
      ))}
      <div className="mc-login-cyano absolute inset-0" aria-hidden="true" />
      <div className="mc-login-cyano-2 absolute inset-0" aria-hidden="true" />
      <div className="mc-login-settle absolute inset-0" aria-hidden="true" />
      <div className="mc-login-tooth absolute inset-0" aria-hidden="true" />

      {/* The title block reads as the drawing's header, so it sits at the top:
          what the drawing is, which plate it is printed on, who took the
          photograph, which revision. It replaces the mono sheet label that used
          to live here and repeated the same two facts. */}
      <dl className="mc-login-fg relative z-10 m-6 grid grid-cols-4 border border-hairline-2 bg-scrim font-mono text-[11px] backdrop-blur-sm">
        <TitleCell label={t('login.plate.drawing')} value="control-plane.auth" />
        <TitleCell label={t('login.plate.plate')} value={t('login.plate.plateValue')} />
        <TitleCell label={t('login.plate.photo')} value={active.credit} />
        <TitleCell label={t('login.plate.rev')} value="C" last />
      </dl>

      {/* Sits high, directly under the title block. */}
      <div className="relative z-10 flex min-h-0 flex-1 items-start px-6">
        <Schematic />
      </div>

      {/* The dimension line is the sheet's footer, so it is a separate drawing
          pinned to the bottom of the column rather than a row inside the
          schematic — inside, its position was whatever the diagram's aspect
          ratio happened to leave it. */}
      <div className="relative z-10 px-6 pb-10">
        <DimensionLine />
      </div>
    </aside>
  );
}

function TitleCell({ label, value, last }: { label: string; value: string; last?: boolean }) {
  return (
    <div className={last ? 'px-2.5 py-1.5' : 'border-r border-hairline-2 px-2.5 py-1.5'}>
      <dt className="text-[9px] tracking-[0.07em] uppercase opacity-60">{label}</dt>
      <dd className="truncate">{value}</dd>
    </div>
  );
}

/**
 * The annotation layer. Machine values (`go · chi · :8705`, `argon2id`) stay in
 * English per `.instructions/donottranslate.md`; the node names are ordinary
 * words and go through `t()`.
 *
 * Geometry note: boxes are 150–160 wide with a 10px text inset, so a sub-line
 * has 140px of usable width. At 7px in a 0.6em-advance mono face that is ~33
 * characters — the longest string here is 27, and the longest translated node
 * title (`AGENT DISTANT`, `AGENTE REMOTO`) is 13 at 10px, or 78px. Nothing
 * overruns its block in any of the six locales.
 */
function Schematic() {
  const { t } = useTranslation('auth');

  return (
    <svg
      viewBox="0 88 640 228"
      preserveAspectRatio="xMidYMid meet"
      aria-label={t('login.plate.schematicLabel')}
      className="mc-login-ink pointer-events-none h-auto w-full"
    >
      <defs>
        <marker id="mc-login-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto">
          <path d="M0 0 L8 4 L0 8 z" fill="currentColor" opacity=".75" />
        </marker>
      </defs>

      <g className="mc-login-box" stroke="currentColor" strokeWidth="1.1">
        <rect x="20" y="152" width="150" height="48" rx="4" />
        <rect x="245" y="258" width="160" height="48" rx="4" />
        <rect x="460" y="120" width="160" height="48" rx="4" />
        <rect x="460" y="186" width="160" height="48" rx="4" />
        <rect x="460" y="252" width="160" height="60" rx="4" />
      </g>
      <rect
        x="245"
        y="132"
        width="160"
        height="88"
        rx="4"
        className="mc-login-box-hi"
        stroke="currentColor"
        strokeWidth="1.7"
      />

      <g fill="currentColor" className="font-mono">
        <text x="30" y="174" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.browser')}
        </text>
        <text x="30" y="189" fontSize="7" opacity=".74">
          {t('login.plate.detail.browserStack')}
        </text>

        <text x="255" y="154" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.controlPlane')}
        </text>
        <text x="255" y="172" fontSize="7" opacity=".74">
          {t('login.plate.detail.controlStack')}
        </text>
        <text x="255" y="186" fontSize="7" opacity=".74">
          {t('login.plate.detail.crypto')}
        </text>
        <text x="255" y="200" fontSize="7" opacity=".74">
          {t('login.plate.detail.cookie')}
        </text>
        <text x="255" y="214" fontSize="7" opacity=".74">
          {t('login.plate.detail.rbac')}
        </text>

        <text x="255" y="280" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.sqlite')}
        </text>
        <text x="255" y="295" fontSize="7" opacity=".74">
          {t('login.plate.detail.storage')}
        </text>

        <text x="470" y="142" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.docker')}
        </text>
        <text x="470" y="157" fontSize="7" opacity=".74">
          {t('login.plate.detail.dockerSocket')}
        </text>

        <text x="470" y="208" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.kubernetes')}
        </text>
        <text x="470" y="223" fontSize="7" opacity=".74">
          {t('login.plate.detail.k8sClient')}
        </text>

        <text x="470" y="274" fontSize="10" fontWeight="600" letterSpacing="0.05em">
          {t('login.plate.node.agent')}
        </text>
        <text x="470" y="289" fontSize="7" opacity=".74">
          {t('login.plate.detail.agentTransport')}
        </text>
        <text x="470" y="303" fontSize="7" opacity=".74">
          {t('login.plate.detail.agentPorts')}
        </text>
      </g>

      {/* Wire lanes. Every vertical run gets its own x and no two segments
          cross: the Docker and Kubernetes drops sit at x=424 and x=432, and the
          agent's tunnel comes up into the control plane from underneath at
          x=448 → y=240 → x=380, which also draws the outbound connection the
          right way round. They shared the x=432 lane before, so the Kubernetes
          drop and the agent's climb were painted on top of each other between
          y=200 and y=210. */}
      <g fill="none" stroke="currentColor" strokeWidth="1" opacity=".85">
        <path d="M170 176 H245" markerEnd="url(#mc-login-arrow)" />
        <path d="M325 220 V258" markerEnd="url(#mc-login-arrow)" />
        <path d="M405 150 H424 V144 H460" markerEnd="url(#mc-login-arrow)" />
        <path d="M405 176 H432 V210 H460" markerEnd="url(#mc-login-arrow)" />
        <path d="M460 282 H448 V240 H380 V220" markerEnd="url(#mc-login-arrow)" />
      </g>

      {/* Traffic. Same geometry, drawn over the wires as a travelling dash —
          see `mc-login-flow` in app.css, gated on both reduce-motion settings. */}
      <g className="mc-login-flow" aria-hidden="true">
        <path d="M170 176 H245" />
        <path d="M325 220 V258" />
        <path d="M405 150 H424 V144 H460" />
        <path d="M405 176 H432 V210 H460" />
        <path d="M460 282 H448 V240 H380 V220" />
      </g>

      <g stroke="currentColor" strokeWidth=".7" opacity=".55" fill="none">
        <path d="M220 132 L190 104 H40" opacity=".5" />
      </g>
      <text x="40" y="100" fontSize="7" letterSpacing="0.04em" className="mc-login-note font-mono">
        {t('login.plate.boundary')}
      </text>
    </svg>
  );
}

/** The sheet's footer rule, measured end to end like a drawing dimension. */
function DimensionLine() {
  const { t } = useTranslation('auth');

  return (
    <svg
      viewBox="0 0 640 28"
      preserveAspectRatio="xMidYMid meet"
      aria-hidden="true"
      className="mc-login-ink pointer-events-none h-auto w-full"
    >
      <defs>
        {/* A second marker id: two SVGs in one document cannot share one. */}
        <marker
          id="mc-login-arrow-dim"
          viewBox="0 0 8 8"
          refX="7"
          refY="4"
          markerWidth="6"
          markerHeight="6"
          orient="auto"
        >
          <path d="M0 0 L8 4 L0 8 z" fill="currentColor" opacity=".75" />
        </marker>
      </defs>

      <text x="320" y="8" textAnchor="middle" fontSize="7.5" fill="currentColor" opacity=".74" className="font-mono">
        {t('login.plate.dimension')}
      </text>
      <g stroke="currentColor" strokeWidth=".7" opacity=".55" fill="none">
        <path d="M20 18 H620" markerStart="url(#mc-login-arrow-dim)" markerEnd="url(#mc-login-arrow-dim)" />
        <path d="M20 12 V24" />
        <path d="M620 12 V24" />
      </g>
    </svg>
  );
}
