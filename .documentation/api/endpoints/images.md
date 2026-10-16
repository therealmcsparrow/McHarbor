# Images and registries

15 endpoints: 9 for images on a Docker host, 6 for the registry credentials used to reach
private ones.

| Key | Covers |
|---|---|
| `images.view` | List, inspect, history |
| `images.manage` | Pull, import, export, tag, prune, and `?reveal=true` on inspect and history |
| `images.delete` | Remove |
| `registry.view` | List and read registry rows |
| `registry.manage` | Add, edit, delete, test a registry |

## Things worth knowing first

**Pull is synchronous.** `POST /api/images` blocks for the duration of the pull and answers
once. There is no image-pull stream; the streaming install flows live in the app store.

**Import and export move tar archives.** `POST /api/images/import` takes a multipart `file`;
`GET /api/images/{id}/export` streams a tar back as a download. Neither is bound by the 8 MiB
JSON body limit.

**Inspect and history mask credentials.** `Config.Env` values and the values in `ENV` and
build-argument history lines whose name or value looks like a credential come back as
`••••••••`. `?reveal=true` returns them to a caller holding `images.manage` (otherwise `403`)
and writes a `reveal_env` audit entry at warning severity. Export is not masked, so it needs
`images.manage` too.

**An image id may be a digest, a short id, or a `repo:tag`** wherever the Docker daemon
accepts all three.

**A registry's password is never returned.** `GET /api/registries/{id}` omits it by design.
Sending `password` on an update replaces it; omitting the field leaves it alone.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/images` | `images.view` | Returns all images | `?all=` |
| `POST` | `/api/images` | `images.manage` | Pulls an image from a registry | — |
| `POST` | `/api/images/import` | `images.manage` | Loads an image from an uploaded tar archive | multipart: `file` |
| `POST` | `/api/images/prune` | `images.manage` | Removes unused images | — |
| `DELETE` | `/api/images/{id}` | `images.delete` | Removes an image | `?force=`, `?noprune=` |
| `GET` | `/api/images/{id}` | `images.view` | Returns detailed image information, credential-looking env values masked | `?reveal=` (`images.manage`) |
| `GET` | `/api/images/{id}/export` | `images.manage` | Streams an image as a tar archive download | — |
| `GET` | `/api/images/{id}/history` | `images.view` | Returns the history of an image, credential-looking ENV and build-argument values masked | `?reveal=` (`images.manage`) |
| `POST` | `/api/images/{id}/tag` | `images.manage` | Tags an image with a new repository and tag | — |
| `GET` | `/api/registries` | `registry.view` | Returns a paginated list of registries | — |
| `POST` | `/api/registries` | `registry.manage` | Adds a new registry | — |
| `DELETE` | `/api/registries/{id}` | `registry.manage` | Removes a registry | `{id}` |
| `GET` | `/api/registries/{id}` | `registry.view` | Returns a single registry (password omitted) | `{id}` |
| `PUT` | `/api/registries/{id}` | `registry.manage` | Updates an existing registry | `{id}` |
| `POST` | `/api/registries/{id}/test` | `registry.manage` | Tests the registry connection | `{id}` |


## Request bodies

**`POST` `/api/images`**: `image`, `tag`?

**`POST` `/api/images/{id}/tag`**: `repo`, `tag`

**`POST` `/api/registries`**: `name`, `url`, `username`, `password`

**`PUT` `/api/registries/{id}`**: `name`?, `url`?, `username`?, `password`?, `isDefault`?

## Related

- [Containers](./containers.md): what runs the images
- [Scans](./security.md): vulnerability scanning of an image
- [Images screen](../../screens/images/README.md)
