# Dashboard and Layout

McHarbor's dashboard and shell UI are built around a persistent application layout
and a widget-driven dashboard system.

## App Layout

The authenticated application shell provides:

- sidebar navigation
- sticky header
- environment selector
- search / command palette entry
- main outlet content area
- footer

## Dashboard Widget Model

The dashboard uses a widget registry plus persisted layout state.

Core parts:

- widget implementations under `src/frontend/widgets/`
- registry in `src/frontend/modules/dashboard/widgets/registry.ts`
- widget sync hook in `src/frontend/modules/dashboard/hooks/useWidgetSync.ts`
- persisted layout store in `src/frontend/modules/dashboard/stores/`

## Widget Registry

The widget registry defines:

- widget IDs
- labels and descriptions
- categories
- icons
- default and minimum sizes
- lazily loaded widget components

Widget categories include:

- resources
- host
- metrics
- monitoring
- operations
- kubernetes

## Built-In Widgets

Examples of built-in widget coverage:

- container, image, volume, and network summaries
- host information
- CPU, memory, and I/O charts
- activity feed
- container lists
- stack status
- resource donuts
- alert and vulnerability summaries
- Kubernetes pod and deployment status

## Widget Sync Flow

The dashboard widget sync hook:

1. fetches widget definition metadata from the backend
2. determines which built-in widgets are enabled
3. updates the widget registry store
4. prunes unavailable widgets from persisted layouts

## Layout Persistence

Dashboard layouts are persisted client-side so each user can retain their preferred
widget arrangement. The UI supports drag-and-drop and responsive sizing.

## Backend Support

The backend widgets module seeds built-in widget definitions and exposes API routes
for listing, installing, updating, and uninstalling widgets.
