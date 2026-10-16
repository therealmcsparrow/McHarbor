# Docker: Fields

Every field on the Docker screen: the registry list and the form behind it, and the ten
groups Docker Info is reported in, from server version down to daemon labels.

## Columns

| Column | What it is |
|---|---|
| Name | Your label |
| URL | Registry endpoint |
| Username | Account used |
| Default | Whether unqualified pulls use it |
| Created | When it was added |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Your label for the registry |
| URL | text | Registry endpoint |
| Username | text | Account |
| Password | secret | Encrypted at rest |
| Default registry | toggle | Used for an unqualified pull |

## Docker Info groups

| Group | Fields |
|---|---|
| **Server** | Version, API version, minimum API version, Git commit, Go version, OS, architecture, kernel version, hostname, server id |
| **Resources** | CPUs, memory |
| **Hardware** | GPU, network adapters, attached devices |
| **Storage** | Storage driver, Docker root dir, driver status |
| **Runtime** | Cgroup driver, cgroup version, default runtime, runtimes, logging driver, isolation |
| **Plugins** | Volume, network and log plugins |
| **Object counts** | Containers (running, paused, stopped) and images |
| **Security** | Security options |
| **Swarm** | Active or inactive, node id, managers, nodes |
| **Labels** | Daemon labels, or a statement that there are none |

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Docker](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
