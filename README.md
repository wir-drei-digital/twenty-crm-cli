# twentycrm

A command-line client for the [Twenty](https://twenty.com) CRM REST API, built for agents and
scripts: JSON in, JSON out, one static binary with no runtime dependencies. Every object of your
workspace, custom ones included, becomes a command: the CLI reads the workspace's data model at
runtime, so a field added in Twenty's UI is usable at once, without a new release. stdout carries
the API response and nothing else; every failure leaves one line of JSON on stderr. Nothing is
deleted for good, changed in bulk or restructured without `--force`.

Not affiliated with or endorsed by Twenty.

twentycrm is tested against Twenty 2.27 and works with Twenty Cloud and self-hosted instances
alike. The binary is called `twentycrm` because Twenty's own `twenty-sdk` package installs a binary
called `twenty`.

## Install

```sh
go install github.com/wir-drei-digital/twenty-crm-cli/cmd/twentycrm@latest
```

Or download a prebuilt binary from
[GitHub Releases](https://github.com/wir-drei-digital/twenty-crm-cli/releases):

| OS      | Architectures | Archive   |
| ------- | ------------- | --------- |
| Linux   | amd64, arm64  | `.tar.gz` |
| macOS   | amd64, arm64  | `.tar.gz` |
| Windows | amd64         | `.zip`    |

Each release also ships `checksums.txt`. Unpack the archive and put `twentycrm` on your `PATH`.

## Setup (once per workspace)

1. In Twenty, under Settings, Roles, create a role for the agent with the object permissions it
   needs and no settings permissions, unless it should change the data model (Data Model) or
   webhooks (API Keys & Webhooks).
2. Under Settings, APIs & Webhooks, create an API key with that role and an expiry. Copy it: Twenty
   shows it once.
3. On each machine, run `twentycrm init`. It asks for the base URL and the key (hidden input),
   checks both with read-only calls, shows the workspace, the number of objects, the company count
   and the key's expiry, asks whether to switch on read-only mode, and saves everything, including
   the workspace model. When the URL or the key does not work, it saves nothing. Without a
   terminal, use `config set` instead and delete the key file afterwards:

   ```sh
   twentycrm config set base-url https://crm.example.com
   twentycrm config set api-key < key.txt
   ```

   Or set the environment variables `TWENTY_BASE_URL` and `TWENTY_API_KEY`.
4. Note the key's expiry date. `twentycrm auth status` shows it and flags it 14 days ahead.

The base URL of Twenty Cloud is `https://api.twenty.com`. For a self-hosted Twenty it is the
address you open in the browser, without a path: `https://crm.example.com`, not
`https://crm.example.com/objects/companies`. A URL copied from the address bar is refused with the
origin to use instead.

Check the result:

```sh
twentycrm auth status   # workspace, key ID and expiry, offline
twentycrm schema        # the objects of your workspace, read live
```

## Usage

```
twentycrm <object> <verb> [id] [flags]
```

`<object>` is the object's plural name in kebab case: `companies`, `people`, `note-targets`,
`workspace-members`, or a custom object such as `invoices`. The API spelling (`noteTargets`) and
the singular (`company`) work too, unless they would collide with a built-in command or another
object. `twentycrm schema` lists the objects of your workspace. The help of every object command
(`twentycrm companies list --help`) lists the object's fields with their types, the allowed values
of select fields and the relation targets.

The verbs are the same for every object:

| Command | Request | Class | Flags and notes |
| --- | --- | --- | --- |
| `list` | `GET /rest/<plural>` | `read` | `--filter`, `--order-by`, `--limit`, `--depth`, `--starting-after`, `--ending-before`, `--all`, `--max-pages` |
| `get <id>` | `GET /rest/<plural>/<id>` | `read` | `--depth` |
| `group-by` | `GET /rest/<plural>/groupBy` | `read` | `--group-by` (required), `--aggregate`, `--filter`, `--order-by`, `--limit`, `--view-id`, `--include-records-sample`, `--order-by-for-records` |
| `find-duplicates` | `POST /rest/<plural>/duplicates` | `read` | `--data` `{"ids":[...]}` or `{"data":[...]}`, `--depth` |
| `create` | `POST /rest/<plural>` | `write` | `--data` object, `--upsert`, `--depth` |
| `batch-create` | `POST /rest/batch/<plural>` | `write` | `--data` array of 1 to 60 objects, `--upsert`, `--depth` |
| `update <id>` | `PATCH /rest/<plural>/<id>` | `write` | `--data` object, `--depth` |
| `delete <id>` | `DELETE /rest/<plural>/<id>?soft_delete=true` | `write` | moves the record to the trash |
| `restore <id>` | `PATCH /rest/restore/<plural>?filter=id[eq]:<id>` | `write` | `--depth`; answers with an array |
| `update-many` | `PATCH /rest/<plural>?filter=...` | `bulk` | `--filter` (required), `--data` object, `--depth` |
| `delete-many` | `DELETE /rest/<plural>?filter=...&soft_delete=true` | `bulk` | `--filter` (required) |
| `restore-many` | `PATCH /rest/restore/<plural>?filter=...` | `bulk` | `--filter` (required), `--depth` |
| `merge` | `PATCH /rest/<plural>/merge` | `bulk`; `read` with `--dry-run` | `--data` `{"ids":[...],"conflictPriorityIndex":N}`, `--dry-run`, `--depth` |
| `destroy <id>` | `DELETE /rest/<plural>/<id>?soft_delete=false` | `destroy` | permanent |
| `destroy-many` | `DELETE /rest/<plural>?filter=...&soft_delete=false` | `destroy` | `--filter` (required), permanent |

- `<id>` is a UUID. Anything else is a usage error, so no argument can add a path segment or
  widen a filter.
- `restore <id>` is `restore-many` limited to that one ID: Twenty 2.27 answers
  `PATCH /rest/restore/<plural>/<id>` with 400. Its response is therefore
  `{"data":{"restore<Plural>":[...]}}`, an array holding the restored record, empty when nothing
  matched.
- `--depth` takes `0` (the record only, Twenty's default) or `1` (with its direct relations), and is
  sent only when given. `--limit` takes 1 to 200. `--upsert` sends `upsert=true`.
- `--dry-run` sets `"dryRun": true` in the merge body. Only the flag makes a merge `read`-class; a
  `dryRun` field inside `--data` relaxes nothing.
- The query flags map to Twenty's parameter names (`--order-by` to `order_by`, `--starting-after`
  to `starting_after`, `--group-by` to `group_by`, and so on). Their values pass through unchanged.

Examples:

```sh
twentycrm companies list --filter 'name[ilike]:"%acme%"' --depth 0
twentycrm companies list --all
twentycrm people get 3f2a1c9e-8b7d-4e6f-a5c4-1b2d3e4f5a6b
twentycrm companies create --data @company.json
```

Link a note to a company through `note-targets` (field names of a standard Twenty 2.27 workspace;
`twentycrm schema note-targets` shows yours):

```sh
twentycrm notes create --data '{"title":"Call","bodyV2":{"markdown":"Discussed the offer."}}'
twentycrm note-targets create --data '{"noteId":"<note id>","targetCompanyId":"<company id>"}'
```

Change every record a filter matches. That is a bulk change, so it needs `--filter` and `--force`:

```sh
twentycrm people update-many --filter 'jobTitle[eq]:CEO' --data '{"jobTitle":"Chief Executive Officer"}' --force
```

`delete` moves a record to the trash, and `restore` brings it back. `destroy` deletes it for good
and needs `--force`:

```sh
twentycrm companies delete 3f2a1c9e-8b7d-4e6f-a5c4-1b2d3e4f5a6b
twentycrm companies restore 3f2a1c9e-8b7d-4e6f-a5c4-1b2d3e4f5a6b
twentycrm companies destroy 3f2a1c9e-8b7d-4e6f-a5c4-1b2d3e4f5a6b --force
```

Preview a merge of two duplicates without changing anything:

```sh
twentycrm companies merge --dry-run --data '{"ids":["<id>","<id>"],"conflictPriorityIndex":0}'
```

Find your way around:

```sh
twentycrm schema companies                          # the fields of one object as JSON, read live
twentycrm commands --json                           # the machine-readable catalog
twentycrm metadata objects list                     # Twenty's metadata API (needs the Data Model permission)
twentycrm api GET rest/companies --query limit=1    # raw request; the guardrails still apply
```

### Filters and order

`--filter` and `--order-by` use Twenty's own syntax and reach Twenty exactly as given, encoded
once. The field names below are examples; `twentycrm schema <object>` lists the real ones.

- A condition is `field[comparator]:value`. Commas join conditions that must all match;
  `or(...)` and `not(...)` combine them: `or(stage[eq]:LEAD,employees[gt]:50)`.
- Comparators: `eq`, `neq`, `in`, `containsAny`, `is`, `gt`, `gte`, `lt`, `lte`, `startsWith`,
  `endsWith`, `like`, `ilike`. `in` and `containsAny` take a list (`status[in]:[DRAFT,SENT]`);
  `is` takes `NULL` or `NOT_NULL`.
- A composite field takes a dot: `emails.primaryEmail[eq]:ana@example.com`.
- Quote a value that holds commas or spaces: `name[ilike]:"%Zürich, AG%"`. `like` and `ilike`
  use `%` as the wildcard.
- `--order-by` is a comma-separated list of `field[direction]` with the directions
  `AscNullsFirst`, `AscNullsLast`, `DescNullsFirst` and `DescNullsLast`:
  `createdAt[DescNullsLast],name`. A field without a direction sorts `AscNullsFirst`.

### Metadata

`twentycrm metadata <kind> <verb> [id]` reaches Twenty's metadata API for the kinds `objects`,
`fields`, `views`, `view-fields`, `view-filters`, `view-sorts`, `view-groups`,
`view-filter-groups`, `page-layouts`, `page-layout-tabs`, `page-layout-widgets`, `webhooks` and
`api-keys`. `list` (with `--limit` 1 to 1000, the cursor flags, `--all` and `--max-pages`) and
`get <id>` are `read`-class; `create`, `update <id>` and `delete <id>` are `admin`-class and need
`--force`. Deleting a field or an object removes its data for good. `api-keys create`, `update` and
`delete` are blocked (see [Guardrails](#guardrails)). `objects` and `fields` need the Data Model
permission on the key's role; `schema` does not.

### Bodies, output and paging

- `--data` takes a JSON literal, `@file.json` or `-` for stdin, up to 20 MB; a file or stdin is
  read no further than that. A UTF-8 byte order mark is stripped; UTF-16 is refused with the fix in
  the message. The body is sent as given.
- The response goes to stdout untouched, with a newline added when it lacks one; an empty 2xx body
  prints nothing. `--output <file>` writes it to a file instead. The file is opened before the
  request, so a path that cannot be written is a usage error and nothing is sent. If writing still
  fails after Twenty accepted the call, the response goes to stdout and the exit is 1 with kind
  `output_failed`.
- `--all` (on `list` and `metadata <kind> list`) follows `pageInfo.endCursor` while
  `pageInfo.hasNextPage` is true, 200 records per page (1000 for metadata) unless `--limit` says
  otherwise, and prints one JSON array of the merged rows: the only transformation the CLI
  performs. `--starting-after` sets the starting point; `--ending-before` cannot be combined with
  it. `--max-pages` (default 100) caps it; hitting the cap with data remaining prints the partial
  array and exits 1 with kind `incomplete`. A page that says there is more but carries no cursor
  stops the walk with kind `server`, after printing the rows collected so far.

Global flags: `--force`, `--output`, `--timeout` (per attempt, default 30s) and `--verbose`
(method, path, status and response size on stderr, never the key). Shell completion comes from
`twentycrm completion`.

## The workspace model

Twenty has no fixed API: each workspace generates its own from its data model. twentycrm reads that
model from `GET /rest/open-api/core`, the workspace's OpenAPI document, which any valid key may read
whatever its role. From it the CLI keeps, per object, the command name, the plural and singular
names and the description, and per field the type and format, the allowed values of select fields,
whether it is required on create or read-only, the subfields of composite fields, and the target of
a relation.

The model is cached in `<user cache dir>/twentycrm/` (`~/.cache/twentycrm` on Linux,
`~/Library/Caches/twentycrm` on macOS, `%LocalAppData%\twentycrm` on Windows), one file per base URL
and workspace, written `0600` inside a `0700` directory. It holds no key.

- Building the command tree never touches the network.
- When a command's first word is not a built-in command, and the cache is missing, older than 24
  hours, or does not know the word, the CLI refreshes the model once and tries again. If the
  refresh fails, a stale cache that knows the word is used. An unreadable cache counts as missing.
- Every `schema` call reads the model live and rewrites the cache.
- Built-in commands, `--help` without a command and shell completion never refresh.
- Without a configured key there are no object commands, and an unknown command answers with the
  setup hint.

A built-in command wins over an object with the same kebab-case name. The built-in names are
`version`, `commands`, `schema`, `api`, `config`, `auth`, `init`, `metadata`, `help` and
`completion`. A shadowed object appears in the catalog with `"shadowed_by": "<command>"` and stays
reachable through `api`.

## Guardrails

### Risk classes

| Class | Commands | Gate |
| --- | --- | --- |
| `read` | `list`, `get`, `group-by`, `find-duplicates`, `merge --dry-run`, `schema`, `metadata ... list/get` | none; allowed in read-only mode |
| `write` | `create`, `batch-create`, `update`, `delete`, `restore` | blocked in read-only mode |
| `bulk` | `update-many`, `delete-many`, `restore-many`, `merge` | `--force` |
| `destroy` | `destroy`, `destroy-many` | `--force` |
| `admin` | `metadata ... create/update/delete` | `--force` |
| blocked | `metadata api-keys create/update/delete` | refused always |

A single `delete` needs no `--force`: the record goes to the trash and `restore` brings it back,
which makes it easier to undo than an `update`, which is not gated either. `delete` always sends
`soft_delete=true`; only `destroy` and `destroy-many` send `soft_delete=false`.

The API key commands are blocked because Twenty lets only a signed-in user manage keys, a new key
would appear once in the response and so in an agent's transcript, and revoking the CLI's own key
would cut it off. They stay in the command tree and in the catalog with `"blocked": true` and the
reason.

### Local checks

Before anything is sent, in this order, each a usage error with exit 2:

1. Argument and body shape: a UUID for `<id>`; `--depth` 0 or 1; `--limit` in range; `--data`
   valid JSON of the shape the verb takes (an object; an array of 1 to 60 objects for
   `batch-create`; for `merge` an object with an `ids` array of at least two IDs; for
   `find-duplicates` an object with `ids` or `data`); `--data` present where the verb has a body.
2. A required `--filter` that is missing or blank.
3. A blocked command.
4. Read-only mode, for anything but `read`.
5. `--force` for `bulk`, `destroy` and `admin`. The message names the command, its class and why
   it needs the flag.

`--force` is a per-call flag with no environment variable or config setting. The CLI never splits a
batch: the parts would no longer succeed or fail together.

### Read-only mode

`TWENTY_READ_ONLY=1` (or `true`, `yes`, `on`, in any case), or `twentycrm config set read-only
true`, allows `read`-class calls only, including through `api`. The environment variable can only
switch it on: `0`, `false`, `no` and `off` leave the config file's setting as it is, and `twentycrm
config unset read-only` switches off what the config file set. Any other value is an error that
names the accepted ones, and only `version`, `help` and `config` run until it is fixed, so a typo
never leaves writes enabled.

### The `api` escape hatch

`twentycrm api <METHOD> <path>` sends a raw request to a path relative to the base URL, with
`--data`, `--query k=v` and `--header k:v` (both repeatable). Methods: `GET`, `POST`, `PATCH`,
`PUT`, `DELETE`. It is not a bypass:

- The path must start with `rest/`. GraphQL is refused. Every path segment must be non-empty, not
  `.` or `..`, and free of `%`, whitespace and backslashes, so no spelling of a path dodges its
  class. Query parameters come from `--query`, never from the path. `soft_delete` and `filter` may
  each appear once and must be spelled exactly so (not `Filter`), and no query key may contain `[`
  or `]`: Twenty would not read such a filter and would act on every record.
- `Authorization` and the method-override headers (`X-HTTP-Method-Override`, `X-HTTP-Method`,
  `X-Method-Override`) cannot be set. `GET` and `DELETE` take no `--data`.
- The path takes its class from Twenty 2.27's own routing, independent of the workspace model, with
  the same gates as the commands:
  - `rest/apiKeys` and `rest/metadata/apiKeys`: `GET` is `read`; anything else is blocked.
  - `rest/metadata`, `rest/webhooks` and `rest/open-api`: `GET` is `read`; anything else is
    `admin`.
  - Every other path is a record path. Twenty reads `rest/<o>`, `rest/batch/<o>`,
    `rest/<o>/groupBy`, `rest/<o>/duplicates`, `rest/<o>/merge` and `rest/restore/<o>` as a whole
    collection, and `rest/<o>/<id>` as one record when `<id>` is a UUID. Longer paths, such as
    `rest/restore/<o>/<id>`, and a second segment that is not a UUID are invalid.
  - `GET` is always `read`.
  - `POST` to `rest/batch/<o>` is `write`, to `.../duplicates` `read`; any other `POST` is `write`,
    or `admin` on an invalid path.
  - `PATCH` to `rest/<o>/merge` is `bulk`.
  - Otherwise `PATCH` and `PUT` (restoring included) are `write` for one record and `bulk` for a
    collection, which needs `--query filter=...`.
  - `DELETE` of one record is `write` when `soft_delete` is exactly `true`, else `destroy`. `DELETE`
    of a collection is `bulk` or `destroy` the same way and needs `--query filter=...`.
  - A change to an invalid path is `admin`.

  So `api PATCH rest/batch/companies` is an update of every company, not a batch call: it needs
  `--query filter=...` and `--force`.

The API client itself refuses a non-GET request that carries no class, so no code path can send a
change past the guard.

### What the CLI cannot do

It cannot stop a caller with shell access from passing `--force`; it makes that decision explicit
and visible. The hard boundary is the role assigned to the API key in Twenty (Settings, Roles): it
decides which objects the key can read or change and whether it may touch the data model. Give each
agent a dedicated role with exactly the objects it needs, and switch on read-only mode where
writing is not its job. See [SECURITY.md](SECURITY.md).

## Errors and exit codes

| Exit code | Meaning |
| --- | --- |
| `0` | success |
| `1` | API or network error (any `kind` except `usage`) |
| `2` | usage error, which includes every guardrail refusal; nothing was sent |

Errors are one line of JSON on stderr, never on stdout:

```json
{"kind":"forbidden","error":"GET rest/metadata/objects: HTTP 403: Forbidden resource; hint: objects and fields need the Data Model permission on the key's role; `twentycrm schema` works without it","status":403,"details":{"error":"FORBIDDEN","messages":["Forbidden resource"],"statusCode":403}}
```

`status` is the HTTP status, omitted when there was no response. `details` is the parsed error
body, a string for a body that is not JSON, or `null`. The message is `<METHOD> <path>: HTTP
<status>: <Twenty's first message>`, with `(and N more)` when Twenty sent more, then a hint when one
applies. Branch on `kind`:

| `kind` | Meaning |
| --- | --- |
| `auth` | HTTP 401 |
| `forbidden` | HTTP 403 |
| `not_found` | HTTP 404 or 410 |
| `validation` | HTTP 400, 422 and other 4xx; also a 3xx, which is refused rather than followed (the message names the `Location`) |
| `conflict` | HTTP 409 |
| `rate_limited` | HTTP 429 beyond the retry budget |
| `server` | HTTP 5xx, or a response the CLI cannot read (a model it cannot extract, a list page of an unknown shape) |
| `transport` | a network failure that is safe to retry: the request never reached Twenty, or it could not change anything; also a call cancelled while waiting to retry |
| `outcome_unknown` | a change failed in flight and may or may not have been applied; read the state before retrying |
| `incomplete` | `--all` hit `--max-pages`; stdout holds the partial array |
| `output_failed` | the call succeeded, but the response could not be written; with `--output` it is on stdout instead. Do not repeat a change |
| `usage` | the command was wrong or a guardrail refused it; nothing was sent |

Twenty answers unhandled server errors with HTTP 400, so a `validation` error is not always a
mistake in the request.

Hints are appended for the errors people meet during setup:

- 401 whose message mentions expiry: the key has expired; create a new one in Twenty under
  Settings, APIs & Webhooks and run `twentycrm config set api-key`.
- Any other 401: the key is invalid or revoked; `twentycrm auth status` shows which key is in use.
- 403 on `metadata objects` or `metadata fields`: these need the Data Model permission;
  `twentycrm schema` works without it.
- Any other 403: the key's role does not allow this on this object (Settings, Roles).
- 400 on an object command whose message mentions a field: run `twentycrm schema <object>` for the
  field names.
- 429: Twenty allows 100 requests per minute.

Retries happen inside the CLI; do not add a retry loop of your own.

- HTTP 429 is retried for every call, with exponential backoff from 1 second plus jitter, waiting
  at least as long as a `Retry-After` header asks, within a total budget of 60 seconds. When the
  next wait would exceed the budget, the call fails with `rate_limited`.
- Network failures and HTTP 5xx are retried only for `read`-class calls (which include
  `find-duplicates` and `merge --dry-run`), up to three attempts.
- A connection that could not be opened (a dial or DNS failure) never reached Twenty and is retried
  for every call.
- Any other change that fails in flight is never replayed and ends as `outcome_unknown`.
- `--timeout` limits each attempt (default 30s). Ctrl-C and SIGTERM cancel the request in flight
  and any retry wait.

## Configuration

The config file is `<user config dir>/twentycrm/config.json` (`~/.config/twentycrm/config.json` on
Linux, `~/Library/Application Support/twentycrm/config.json` on macOS,
`%AppData%\twentycrm\config.json` on Windows), written `0600` inside a `0700` directory;
`twentycrm config path` prints the exact location. `twentycrm config set <key>` and `twentycrm
config unset <key>` take these keys:

| Key | Value |
| --- | --- |
| `base-url` | `scheme://host[:port]`, normalised to lower case without a trailing slash; HTTPS except for `localhost`, `127.0.0.1` and `::1` |
| `api-key` | the API key, from stdin only; needs a stored base URL, and prints the workspace ID, the key ID and the expiry |
| `read-only` | `true` or `false` |

- `config set base-url` refuses a URL with a path, a query, a fragment or user info, and shows the
  accepted form. With a key stored for another base URL it refuses and says to run `config unset
  api-key` first; setting the same URL again is a no-op.
- `config set api-key` strips a `Bearer ` prefix and whitespace, and refuses anything that is not a
  Twenty API key (a user's access token, for example) with a message saying what it is.
- `config unset base-url` refuses while a key is stored. `config unset api-key` removes the local
  copy; the key stays valid until it is revoked in Twenty (Settings, APIs & Webhooks).
- An environment variable name or an unknown key is a usage error that names the valid keys.
- After a `set`, a note on stderr says when an environment variable overrides the saved value.
- When the config file is corrupt, or `TWENTY_READ_ONLY` holds a value the CLI does not know, only
  `version`, `help` and `config` run; repair the file or the variable.

Environment variables win over the file:

| Variable | Meaning |
| --- | --- |
| `TWENTY_BASE_URL` | the base URL |
| `TWENTY_API_KEY` | the API key |
| `TWENTY_READ_ONLY` | `1`, `true`, `yes` or `on` (any case) switches read-only mode on; `0`, `false`, `no` and `off` leave the file's setting; any other value is an error. It cannot switch off what the file set |

A key from the config file is only ever sent to the base URL stored with it. When the key comes
from the file and `TWENTY_BASE_URL` names another base URL, every API call is refused with a usage
error saying to set `TWENTY_API_KEY` as well or to unset `TWENTY_BASE_URL`. A key and a base URL
that both come from the environment are used as given: whoever sets both already holds the key.

`twentycrm auth status` prints the configuration as one line of JSON, offline, never the key:

```json
{"mode":"api_key","source":"config","base_url":"https://crm.example.com","base_url_source":"config","workspace_id":"3b8e6a2c-1f4d-4a5e-9c7b-2d1e0f9a8b7c","key_id":"9f1c2e3d-4b5a-4c6d-8e7f-0a1b2c3d4e5f","expires_at":"2027-03-31T00:00:00Z","expires_in_days":186,"expires_soon":false,"expired":false,"read_only":false}
```

| Key | Meaning |
| --- | --- |
| `mode` | `api_key`, or `none` when no key is configured |
| `source` | where the key comes from: `env` or `config` |
| `base_url`, `base_url_source` | the base URL in use and where it comes from |
| `workspace_id`, `key_id` | read from the key |
| `expires_at`, `expires_in_days` | the key's expiry (RFC 3339) and the days left |
| `expires_soon`, `expired` | fewer than 14 days left; already expired |
| `read_only` | whether read-only mode is on |
| `key_error` | why the key could not be read; a key from `TWENTY_API_KEY` is still sent as given |
| `missing` | what blocks calls: `base_url`, `api_key` |
| `hint` | the next step: a missing setting, a base URL the stored key is not bound to, a key that cannot be read, has expired or expires soon |

## Compatibility

twentycrm is tested against Twenty 2.27; `twentycrm version` names the version a release was tested
against. The route grammar of Twenty's REST API has been stable, and the workspace model is read
at runtime, so newer Twenty versions are expected to work. After a Twenty upgrade on your side, run
the read-only live check (see [Development](#development)).

This project follows SemVer. The public API is the command grammar, the flags, the exit codes, the
stderr error schema and the `twentycrm commands --json` schema (`schema_version`). New optional
keys may appear in the catalog under the same `schema_version`, so consumers must ignore keys they
do not know.

## Development

```sh
make build   # go build -o bin/twentycrm ./cmd/twentycrm
make test    # go test ./...
make check   # go vet + tests; CI runs these on Linux, macOS and Windows, plus a CGO-free cross-compile
```

`e2e/smoke_test.go` builds the real binary and drives it against a fake Twenty that serves the
fixture OpenAPI document. `e2e/live_test.go` runs against a real Twenty with your own configuration
(config file or `TWENTY_*` variables) and is skipped unless you ask for it:

```sh
TWENTY_LIVE=1 go test ./e2e -run TestLive -v
```

By default it only reads: `auth status`, `schema companies`, and `companies list --all --depth 0`
against `totalCount`. More checks switch on with more variables:

| Variable | Check |
| --- | --- |
| `TWENTY_LIVE_SELECT_FIELD=<field>` | `schema companies` lists the values of this select field |
| `TWENTY_LIVE_RESTRICTED_KEY=<key>` | with a key whose role lacks the Data Model permission, `schema` works and `metadata objects list` fails with `forbidden` and the hint |
| `TWENTY_LIVE_WRITE=1` | creates a company named `twentycrm-live-<unix time>`, reads, renames, trashes, restores and destroys it, checking each step |

## License

MIT, see [LICENSE](LICENSE).
