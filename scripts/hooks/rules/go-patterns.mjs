// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { inChanged } from '../lib/payload.mjs';

const IGNORED_ERROR = /^\s*_,\s*_\s*=\s/;
const CHECKORIGIN_TRUE = /CheckOrigin:\s*func\([^)]*\)\s*bool\s*\{\s*return\s*true\s*\}/;
const GET_PREFIX = /func\s*\([^)]*\)\s*Get([A-Z]\w*)\s*\(/;
// Anchored to the store/tx receivers so `.Query()` on an *http.Request (every
// handler calls r.URL.Query()) never matches.
const SQL_IN_HANDLER = /\b(?:db|tx|conn|DB)\.(?:Query|QueryRow|Exec)(?:Context)?\(/;
const ERRORF_VW = /fmt\.Errorf\(\s*"[^"]*%v[^"]*"\s*,\s*([A-Za-z_]\w*)\s*\)/;

function basename(path) {
  return path.split(/[\\/]/).pop() ?? '';
}

// `*handler.go` misses handler_propose.go and similar; the glob this repo
// actually wants is "handler" anywhere in the filename.
// The remote agent answers the host over its RPC protocol, never an HTTP client;
// the host logs the agent's message and returns an i18n code, so src/agent is out of scope.
function isAgentFile(path) {
  return /(^|[\\/])src[\\/]agent[\\/]/.test(path);
}

function isHandlerFile(path) {
  return /handler/i.test(basename(path)) && !isAgentFile(path);
}

function isTestFile(path) {
  return /_test\.go$/.test(path);
}

/** check runs every Go pattern against the lines a tool call actually touched. */
export function check({ path, text, changedLines }) {
  const findings = [];
  const lines = text.split('\n');
  const handlerFile = isHandlerFile(path);
  const testFile = isTestFile(path);

  lines.forEach((raw, idx) => {
    const line = idx + 1;
    if (!inChanged(changedLines, line)) return;

    if (IGNORED_ERROR.test(raw)) {
      findings.push({
        line,
        rule: 'go/ignored-error',
        message: 'handle or log the error instead of discarding both return values (`_, _ =`)',
        severity: 'warn',
      });
    }

    const errorfMatch = raw.match(ERRORF_VW);
    if (errorfMatch && errorfMatch[1] === 'err') {
      findings.push({
        line,
        rule: 'go/wrap-with-w',
        message: 'use %w not %v in fmt.Errorf(...) to preserve the error chain',
        severity: 'warn',
      });
    }

    if (CHECKORIGIN_TRUE.test(raw)) {
      findings.push({
        line,
        rule: 'go/checkorigin-true',
        message: 'CheckOrigin always returning true accepts any origin — use httpx.IsAllowedWebSocketOrigin, or a // websocket origin: <reason> marker on the func line',
        severity: 'error',
      });
    }

    if (!testFile) {
      const getMatch = raw.match(GET_PREFIX);
      if (getMatch) {
        findings.push({
          line,
          rule: 'go/get-prefix',
          message: `drop the Get prefix — use ${getMatch[1]}() not Get${getMatch[1]}(), per Effective Go`,
          severity: 'warn',
        });
      }
    }

    if (handlerFile) {
      if (raw.includes('err.Error()')) {
        findings.push({
          line,
          rule: 'go/raw-err-to-client',
          message: 'never return raw err.Error() to the client — log server-side and return response.*Code(w, r, i18n.ErrX)',
          severity: 'error',
        });
      }
      if (SQL_IN_HANDLER.test(raw)) {
        findings.push({
          line,
          rule: 'go/sql-in-handler',
          message: 'no SQL in handlers — move the query into service.go',
          severity: 'error',
        });
      }
    }

    if (raw.includes('// TODO')) {
      findings.push({
        line,
        rule: 'go/todo-marker',
        message: 'use // Stub: not // TODO: for unimplemented work',
        severity: 'warn',
      });
    }
  });

  return findings;
}
