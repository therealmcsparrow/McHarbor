# llama.cpp: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| **Test** cannot connect | `localhost` inside the McHarbor container is the container, not your machine | Use the LAN address of the server, or `host.docker.internal` on Docker Desktop; never `localhost`. |
| Models list is empty | No model is loaded or pulled on the server yet | Load a model on the server first; the list reflects what the server reports. |
| Requests time out on the first call | The server is loading the weights on demand | Pre-load the model, or accept a slow first request and a fast second one. |

**Test** exercises the credential against the real endpoint and reports what came back,
so a green test means the key works and the base URL resolves from inside the McHarbor
container.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i ai-provider-llamacpp
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [llama.cpp](./README.md): what it is and how it works
