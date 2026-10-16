// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Shared shape for the four "status strip" cards — container Health and
 * Uptime, environment Agent health and Agent uptime.
 *
 * They are the same card in two different places, and they used to drift:
 * each is a header, a one-or-two-line summary, a strip of blocks and a footer,
 * so a summary that wrapped in one card and not the other left the two strips
 * at different heights. The agent health card additionally carries a small
 * chart, which made the environment pair taller than the container pair.
 *
 * `STATUS_CARD` fixes both: a full-height flex column inside a stretching grid
 * cell. Each card grows to match its neighbour and no further — there is
 * deliberately no minimum height, so the pair is as tall as its content needs
 * and the environment pair matches the container pair rather than exceeding it.
 *
 * Every card ends with the same two elements, the strip and its footer, and
 * the element above them takes the slack: the summary in three of the cards
 * (`STATUS_CARD_SUMMARY`), and the latency chart in the agent health card,
 * which would otherwise make that one taller than the rest
 * (`STATUS_CARD_FILL`).
 */
export const STATUS_CARD = 'flex h-full flex-col rounded-lg mc-glass p-6';

/** The growing element that pushes the strip and footer to the bottom. */
export const STATUS_CARD_SUMMARY = 'mb-4 flex-1';

/** Wrapper for the loaded content, so the summary above has a flex parent. */
export const STATUS_CARD_BODY = 'flex flex-1 flex-col';

/**
 * The agent health card's latency chart. It sits between the summary and the
 * strip and absorbs the spare height, so that card ends level with the other
 * three instead of standing a chart's worth taller.
 *
 * No minimum height: the chart takes whatever the card has left after the
 * summary, strip and footer, and the card takes its height from its
 * neighbour. A floor here would put the environment pair back above the
 * container pair, which is the thing this was changed to stop.
 */
export const STATUS_CARD_FILL = 'mb-4 flex-1';
