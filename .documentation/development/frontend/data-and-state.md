# Frontend: Data and state

How a page gets its data, reports failures, gates actions and translates its text. Server state
lives in **TanStack Query** and client state in **Zustand**. Never use raw `fetch` with
`useState`. All paths are under `src/frontend/`.

## The API client

`api` from `@core/api/client` is the only HTTP client.

| Method | Use it for |
|---|---|
| `api.get<T>(path, params?)` | A read. `params` become the query string |
| `api.getWithMeta<T>(path, params?)` | A read that also returns `cachedAt` for a PWA-cached answer. Show it with `StaleNotice` |
| `api.post`, `api.put`, `api.patch`, `api.del` | Writes. `post` accepts `{ signal, headers }` |
| `api.putBinary(path, blob)`, `api.download(path, params?)` | Raw bodies |

Paths are relative to `/api`. Every call sends the session cookie and an `Accept-Language` header
from the selected language, so the backend translates its error codes. A `401` outside `/auth/`
redirects to sign-in.

**The client never throws on a failed answer.** It returns the envelope:

```ts
type ApiResponse<T> = { success: boolean; data?: T; error?: string; message?: string; code?: string };
type PaginatedData<T> = { items: T[]; total: number; page: number; per_page: number; total_pages: number };
```

### `assertSuccess`

`assertSuccess` from `@resources/utils/api-mutation` turns the envelope into data or an error:

- On success it returns `data`.
- When the change was held for approval, it throws `ApprovalRequiredError`.
- On any other failure it throws `ApiError`, which carries the HTTP `status`. The message is built
  in this order: the translated `code` (looked up in the namespace named by the code's second
  segment), then `error`, then `message`, then "Operation failed".

`isClientError(err)` is true for a 4xx response. When a module needs to tell one refusal reason
from another, it wraps `assertSuccess` in its own `refusalOf`. That function throws an error that
carries `data.reason` (`modules/reconciler/hooks/refusal.ts` is the pattern). Never hand-roll
`.then((r) => r.data)`. A lint rule and `test/query-envelope.contract.test.ts` both reject it.

## Queries

```ts
// resources/hooks/useContainers.ts
export function useContainers(all = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const uploadActive = useEnvironmentUploadActive(envId);
  return useQuery({
    queryKey: ["containers", envId, all],
    queryFn: () =>
      api.get<ContainerInfo[]>("/containers", { all: String(all), ...(envId ? { env: envId } : {}) })
        .then(assertSuccess),
    refetchInterval: uploadActive ? false : 10_000,
    enabled: !uploadActive,
  });
}
```

- **Query keys.** Start with the resource name, then the environment id, then anything else that
  changes the answer: `['containers', envId, all]`. Put a shared key in a constant (for example
  `SECRETS_QUERY_KEY`) so mutations can invalidate it. A module with several related keys keeps
  them in a `queryKeys.ts` factory beside its hooks (`gitopsKeys`, `updatesKeys`, `stackLinkKeys`,
  `containerBackupKeys`, `userKeys`), so the hook and the invalidation read one definition.
  `test/queryKeys.contract.test.ts` pins every factory's values, because a changed key silently
  stops a mutation from invalidating its query.
- **The environment.** The selected environment is `useEnvironmentStore((s) => s.currentId)`.
  Pass it as `env`, and include it in the key so that switching environments refetches. Outside a
  hook, `envUrl(path)` and `envParams()` from `@core/api/env-url` append it.
- **Defaults** (`main.tsx`): `staleTime` is 10 s, a failed query retries once and never retries a
  4xx, and there is no refetch on window focus.
- **Polling.** Use `refetchInterval`. Pause it while `useEnvironmentUploadActive(envId)` is true.
  Through the remote agent, every container stats call costs about 2 s, so do not poll anything
  per container.
- **The page** renders `<ErrorState>` on `isError`, and a skeleton while loading.

## Mutations

```ts
// resources/hooks/useSecrets.ts
export function useApplySecret() {
  const { t } = useTranslation('secrets');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, resources }: ApplySecretInput) =>
      api.post<SecretApplyResult[]>(`/secrets/${id}/apply`, { resources: resources ?? [] })
        .then((r) => { assertSuccess(r); return r.data ?? []; }),
    meta: { success: t('toast.applied') },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: [SECRETS_KEY] }); },
  });
}
```

The global `MutationCache` in `main.tsx` shows every toast:

- `meta.success` (a string or a function) becomes a success toast.
- A failure becomes an error toast with the server's translated message. An approval hold becomes
  an info toast.
- `meta.error` only **names** the operation. It does not repeat the message.
- Do not add an `onError` whose only job is a toast. That shows the same failure twice. Use
  `onError` only for something the cache cannot do, such as rolling back optimistic state or
  closing a dialog.
- Never call `window.alert` or `window.confirm`. A destructive action goes through
  `MutationConfirmDialog`, which keeps the server's message visible in the open dialog.

## Shared hooks

`resources/hooks/` holds about 106 hooks. Most are a TanStack Query wrapper for one backend
resource, with its types alongside. Look here before writing a query. A hook that two modules need
lives here and is never copied.

| Area | Hooks |
|---|---|
| Docker resources | `useContainers`, `useContainersBulkStats`, `useContainerLogs`, `useContainerShells`, `useImages`, `useNetworks`, `useStacks`, `useStacksForEnvironment`, `useStackLinks`, `useTakeOver`, `useGitRepos`, `useScans`, `useContainerBackups` (and `containerBackups/`) |
| Environment and host | `useEnvironmentList`, `useCurrentEnvironmentActivitySettings`, `useDockerInfo`, `useHostHardware`, `useHostMetrics`, `useMetricsHistory`, `useCapacity`, `useAgentInventory`, `useHostServices`, `useHostUpdates`, `useHostReboot`, `useHostSelfKey`, `useTLS` |
| Realtime | `useDockerEvents` (the SSE event stream, mounted once by the shell; it invalidates caches), `useContainerLogs`, `useHostUpdates` |
| Permissions | `usePermissionGate`, `useTabPermissionGate`, `useVisibleNavItems`, `useElevations` |
| URL state | `useUrlSearchState`, `useUrlNumberState`, `useTabFromUrl` (the active tab as a URL parameter) |
| Commands and keyboard | `useShortcuts`, `useGlobalShortcuts`, `useCommandRegistry` (register command-palette entries while mounted) |
| Layout and appearance | `useHeaderSlot`, `useLayoutMode`, `useResolvedLayout`, `useTheme`, `useAppearanceSettings`, `useBranding`, `useToastSettings` |
| Security and identity | `useUsers`, `useSessions`, `useMFA`, `useSecrets`, `useSecretsList`, `usePolicies`, `usePolicyTester`, `useIdentityAdapters`, `useScimEndpoints`, `usePasswordPolicySettings` |
| Operations | `useAlerts`, `useAlertEvents`, `useIncidents`, `useOnCallQuery`, `useRunbooks`, `useInAppNotifications`, `useStorageLocations`, `useStorageAdapters`, `useBackupProviderRuns`, `useRetentionSettings`, `useComplianceAttestations` |
| Generic helpers | `useReactiveQueryData` (read cached query data without fetching), `useBatchProgressOperation` (the state behind the batch progress dialog), `useStableListKeys` (stable keys for rows without an id), `useFlash` / `useFlashValue` (a value that resets itself, for "Copied"), `useOnline`, `useFocusTrap`, `useLocaleFormat`, `useIdleLogout` |

These hooks belong to the host. A plugin bundle that imports one gets its own copy, and its own copy
of every store behind it. Plugins use `window.McharborHost` instead. See
[Hooks → Host services](../hooks/README.md#host-services-windowmcharborhost).

## Client state: Zustand stores

`resources/stores/` holds UI state only. Stores named `mcharbor-*` persist to `localStorage`.

| Store | Holds |
|---|---|
| `environment` | The selected environment and the cached environment list. This is where `?env=` comes from |
| `language` | The UI language, which is also sent as `Accept-Language` |
| `theme`, `themeCustomizer`, `motion` | Light, dark or system (the default is dark), per-theme token overrides, density, font, and the reduced-motion preference |
| `sidebar`, `navigation`, `layoutMode` | Sidebar collapse and the open section, recents and favourites, and desktop or on-call layout |
| `headerSlot`, `docked-banners`, `demoBannerStore` | Page-header ownership, and dismissed banners |
| `upload-activity` | Uploads in progress per environment. `useEnvironmentUploadActive(envId)` pauses polling and events while one runs |
| `mcharbor-restart`, `toastPreview` | The restart overlay, and the toast preview in settings |

A module that needs its own view state (a view mode, a filter) adds `modules/<x>/stores/`, as
`modules/volumes/stores/volumes-view.ts` does.

## Permissions

The backend enforces permissions. The frontend's job is to be honest: a denied control is
**disabled with a tooltip naming the key**, never hidden. Navigation is the exception, and hides
the entry.

| API | Where | Use it for |
|---|---|---|
| `PERM` | `@core/auth/permissionKeys` | Every key as a constant (`PERM.VOLUMES_MANAGE`). Never write a bare `'volumes.manage'` |
| `permission` prop | `Button`, `ActionButton`, `HeaderActionButton`, `ConfirmDialog` | Gating an action. Pass a key or an any-of list |
| `usePermissionGate(key)` | `@resources/hooks/usePermissionGate` | Anything the buttons do not cover. It returns `{ allowed, denied, reason }`, where `reason` is the translated "Requires …" text |
| `usePermission(keys)`, `useAllPermissions(keys)` | `@core/auth/usePermission` | Any-of and all-of checks |
| `useTabPermissionGate(map)` | `@resources/hooks/useTabPermissionGate` | The denial reason for each tab |
| `<RequirePermission permission fallback>` | `@core/auth/RequirePermission` | Gating a whole panel. The fallback defaults to an access-denied empty state |

A new key needs all five layers in `.claude/rules/permissions.md`. That includes the `PERM` entry,
and the backend `RequirePermission` on the route.

## i18n

Every user-facing string goes through `t(...)`. That covers JSX text, `aria-label`,
`placeholder`, `title`, `alt`, toasts, dialog titles and table headers.

- **Name the namespace:** `useTranslation('volumes')`. For a key in another namespace, use
  `t('actions.cancel', { ns: 'common' })`.
- **Reuse before you add a key.** `common.json` already has `actions.*` (cancel, save, close,
  delete, confirm, refresh…), `labels.*`, `lifecycle.*`, `clipboard.*` and `nav.*`.
- **Add a key to all six locales in the same change**: `en`, `nl`, `de`, `es`, `fr` and `pt`, under
  `core/i18n/locales/<lang>/<namespace>.json`. Write the real translation into the five other
  locales when you author the key; nobody translates later. English may stay only where a native
  speaker would write the same word (a cognate such as `Server`, or a technical token), and each such
  value gets a `locale|namespace|key` line in `.instructions/sanity/baselines/i18n-untranslated.txt`.
  Add the lines by hand. Do not run `sync-locales.mjs --write`, which copies English everywhere and
  reformats the bundles; run `node scripts/sanity/i18n/sync-locales.mjs` without `--write` as the
  parity check.
- **No `defaultValue`.** A missing key must show up as a raw key, not be hidden behind English.
- **Never parse and re-dump a locale file.** Edit the lines.
- `node scripts/sanity/i18n/verify-keys.mjs <files>` checks your keys. A PostToolUse hook runs it
  on every edited file. The same hooks flag an `nl`, `de`, `es`, `fr` or `pt` value that is the
  English text verbatim (`i18n/untranslated`: an error at three or more translatable words, a
  warning at one or two, unless the baseline lists it), and a removed key that nothing references
  any more (`i18n/key-unused`: delete it from all six locales).

The 36 host namespaces load in `core/i18n/i18n.ts`. Widgets and nodes ship their own `i18n/` folder
next to their code. Plugin namespaces are fetched from `/api/plugins/loader/i18n` and merged into
the same i18next instance.

## Utilities

| Need | Use |
|---|---|
| Combining class names | `cn(...)` from `@resources/utils/cn`, which runs `clsx` then `tailwind-merge` |
| Bytes, dates, times, uptime | `formatBytes`, `formatDate`, `formatDateCustom`, `formatDateOnly`, `formatTime`, `formatUptime` from `@resources/utils/format`, with `useLocaleFormat` |
| Date ranges | `@resources/utils/date-range` |
| Cron and readable schedules | `@resources/utils/schedule`, which backs `ReadableScheduleField` |
| Copying to the clipboard | `copyToClipboard`. Without a secure context it returns `manual-required`, and you then show `ManualCopyDialog` |
| Downloads and table export | `saveBlob`, `downloadFile` from `@resources/utils/download`, and `downloadTableExport` from `@resources/utils/table-export` |
| Search matching | `createSearchMatcher`, `matchesSearchFields` from `@resources/utils/search-filter` |
| Status, severity, capacity tones | `statusPalette`, `severity`, `container-state`, `capacity` (`capacityTone`) |
| Redacted environment values | `ENV_REDACTION_MARKER`, `isRedactedValue` from `@resources/utils/env-redaction` |
| A toast outside a mutation | `appToast` from `@resources/utils/toast` |

Shared domain types live in `@core/types/*`. Frontend-only shared types live in
`@resources/types/*`.

## Testing

| Helper | Where | What it does |
|---|---|---|
| `renderWithProviders(ui)` | `test/render.tsx` | Wraps the UI in a fresh `QueryClient` (no retry) and the tooltip provider |
| `server` | `test/server.ts` | The MSW server. Add handlers with `server.use(http.get(...))`. An unmocked request fails the test |
| `grantPermissions(keys \| '*')` | `test/permissions.ts` | Grants keys through the same bridge the app reads. They are reset after each test |
| `describeDestructiveDialog(case)` | `test/destructive.tsx` | The four assertions every destructive control owes |
| `floor.test.ts` | `test/` | The minimum test count per guarded file. The floor only grows |

Tests use the real English bundles, so a missing key fails them. Run
`cd src/frontend && npx vitest run <files> && npx tsc -b`.

## Related

- [Layout](./layout.md)
- [Components](./components.md)
- [Hooks](../hooks/README.md)
- [Development workflow → Testing](../workflow/README.md#testing)
