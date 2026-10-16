# Box: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test fails with a 401 straight after consent | The app was re-created, or the secret was rotated in Box | Re-enter the client secret and run consent again, because the stored refresh token is bound to the old secret. |
| Uploads work but nothing is visible in Box | The destination is writing into a folder the consenting user cannot see | Clear `folder_id` to test against the root, then set the id of a folder the account actually owns. |
| App-only auth reports "subject not found" | `box_subject_type` and `box_subject_id` disagree | Use `enterprise` with the enterprise ID, or `user` with a user ID. The pair has to match. |

Run **Test** on the destination after any change. It performs a full write, change and
delete round trip, so a green test also proves the credentials can clean up after
themselves, which is what retention needs.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i storage-box
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Box](./README.md): what it is and how it works
