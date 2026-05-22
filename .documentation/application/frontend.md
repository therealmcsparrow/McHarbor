# Frontend Architecture

The frontend is a React SPA built with Vite. It is optimized for authenticated
operations across Docker, Kubernetes, workflows, dashboards, and platform settings.

## Entry Point

The frontend entry point is `src/frontend/main.tsx`.

It initializes:

- React root
- TanStack Query client
- mutation success and error toasts
- theme provider
- tooltip provider
- auth provider
- global app CSS

## Routing

The route tree in `src/frontend/core/router.tsx` uses React Router.

There are three main route groups:

- root route for auth bootstrap behavior
- auth layout routes:
  - `/login`
  - `/setup`
- app layout routes:
  - dashboards
  - Docker resources
  - Kubernetes resources
  - workflows
  - security
  - settings
  - notifications
  - store

## Application Shell

The authenticated UI uses `AppLayout`, which provides:

- sidebar navigation
- header with environment selector and search
- routed page content
- persistent footer
- command palette integration

## State Model

The frontend uses a split between server state and client state:

### Server state

- TanStack Query
- API-backed lists, detail pages, dashboard data, workflow data, settings, and runtime information

### Client state

- Zustand stores
- environment selection
- theme and language
- dashboard layout
- widget registry
- header slot state
- workflow editor state

## Module Structure

Feature modules live under `src/frontend/modules/`.

Common structure:

- `pages/`
- `components/`
- `hooks/`

Shared code lives under:

- `src/frontend/resources/`
- `src/frontend/core/`

Widget implementations live under:

- `src/frontend/widgets/`

Workflow node definitions live under:

- `src/frontend/nodes/`

## UI Stack

- Tailwind CSS 4
- Radix primitives through local wrapper components
- TanStack Table
- Recharts
- react-grid-layout
- xterm for terminal UI
- CodeMirror for editor surfaces
- sonner for toasts

## Frontend Loading Strategy

- Most pages are lazy-loaded.
- Suspense fallback uses a centered spinner.
- The router keeps the main bundle smaller by loading many feature pages on demand.

## Production Delivery

In production, the frontend is built to `dist/` and then copied into the backend
container image as `./static`. The Go backend serves those files directly.
