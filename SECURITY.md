# Security

## What an API key is

twentycrm authenticates with a Twenty API key: a JWT that Twenty signs. A key belongs to exactly one
workspace. It carries the expiry chosen when it was created (a key created without one is valid for
100 years), and Twenty applies the role assigned to it. Anyone who holds the key can do what its
role allows, from any machine, until it expires or is revoked. Twenty shows a key once, when it is
created. It is revoked in Twenty under Settings, APIs & Webhooks.

`twentycrm auth status` reads the workspace ID, the key ID and the expiry from the key itself,
offline, without sending it anywhere. It never prints the key.

Limit what a key can do:

- Assign it a role with exactly the objects the work needs (Settings, Roles), and no settings
  permissions unless it should change the data model (Data Model) or webhooks (API Keys &
  Webhooks).
- Give it an expiry. `twentycrm auth status` flags a key 14 days before it expires.
- Create one key per machine or runner. One of them can then be revoked without touching the
  others, which one shared key cannot offer.

## Where the key lives

Either in the config file or in `TWENTY_API_KEY`; the environment wins.

- The config file is at the path `twentycrm config path` prints. It is written `0600` inside a
  `0700` directory, through a temp file and a rename, so a crash never leaves half a file. On
  Windows it gets only the user-scoped protection of `%AppData%`.
- `twentycrm config set api-key` reads the key from stdin, and `twentycrm init` asks for it with
  hidden input. The key never comes from the command line, which other processes on the machine
  can read.
- A `Bearer ` prefix and surrounding whitespace, as copied from a curl example, are removed before
  the key is parsed, stored or sent.
- `twentycrm config unset api-key` removes the local copy and nothing else: the key stays valid
  until it is revoked in Twenty.
- The workspace model cache (`<user cache dir>/twentycrm/`) holds the base URL, the workspace ID
  and the objects and fields of the workspace, never the key.

## The base URL binding

A key from the config file is only ever sent to the base URL stored with it. When such a key meets
a `TWENTY_BASE_URL` that points anywhere else, every API call is refused with a usage error, before
anything is sent. `config set base-url` refuses to move a stored key to another base URL; unset the
key first. A key and a base URL that both come from the environment are used as given: whoever
sets both already holds the key.

The base URL must use HTTPS, except for the loopback hosts `localhost`, `127.0.0.1` and `::1`.

## What the CLI does not do

- It does not follow redirects. A redirect could strip HTTPS off the key or replay a change on
  another host; it is reported as an error that names the `Location`.
- It does not log the key. `--verbose` prints the method, the path, the status and the response
  size, never a header.
- It does not take a key from argv, and `twentycrm api` cannot set `Authorization` or the
  method-override headers `X-HTTP-Method-Override`, `X-HTTP-Method` and `X-Method-Override`.
- It does not create, change or revoke API keys. `metadata api-keys create`, `update` and `delete`
  are refused before any network I/O, as commands and through `api`, whatever the flags.
- It does not delete a record permanently, change or trash records in bulk, merge records or
  change the data model without `--force`.
- It does not talk to any host but the configured base URL.
- It does not verify the key's signature. It cannot: the secret is on the server, so only Twenty
  can. The CLI only reads the key's claims.

The guardrails cannot stop a caller with shell access from passing `--force`; they make that
decision explicit and visible. The hard boundary is the role assigned to the key in Twenty.

## Reporting a vulnerability

Report security issues privately through GitHub Security Advisories on this repository (Security,
Report a vulnerability). Please do not open a public issue. Include `twentycrm version`, the steps
to reproduce and the impact you see.
