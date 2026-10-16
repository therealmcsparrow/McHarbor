# Telegram: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `chat not found` | The bot has never been talked to, or was never added to the group | Send the bot a message first; for a group, add the bot as a member. |
| `can't parse entities` | A message contains Markdown control characters and `parse_mode` is set | Clear `parse_mode`. |
| Nothing arrives in a channel | The bot is not an administrator of the channel | Promote the bot; channels require admin rights to post. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-telegram
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Telegram](./README.md): what it is and how it works
