# Twenty CRM CLI: Design

**Date:** 2026-09-25
**Status:** Approved (design approved section by section in conversation on 2026-09-25)

## Goal

A single-binary CLI, written in Go, that lets AI agents and humans read and change the records of a
[Twenty](https://twenty.com) CRM workspace from the shell: companies, people, tasks, notes, their
links, and any custom object. It works against Twenty Cloud and self-hosted instances alike. Nothing
is deleted for good, changed in bulk or restructured without an explicit, visible decision. Runs on
Linux, macOS and Windows with no runtime dependencies. Public open-source project under the MIT
license.

- Binary: `twentycrm`. Twenty's own `twenty-sdk` npm package installs a binary called `twenty` (for
  building Twenty apps, not for working with records), and the npm package `twenty` announces "a new
  command-line interface" as coming soon, so `twenty` is taken.
- Module: `github.com/wir-drei-digital/twenty-crm-cli`.
- Not affiliated with or endorsed by Twenty; the README says so.

It is a sibling of `bexio-cli`, `klara-cli`, `cash-ctrl-cli`, `grav-cms-cli` and `google-ads-cli` and
keeps their stack and contracts on purpose, so an agent that knows one of them knows this one.

## What was verified before designing this

Checked on 2026-09-25 against Twenty's source at tag `twenty/v2.27.0` (the version our instance
runs), the public docs, and the unauthenticated endpoints of `https://crm.wirdrei.digital`. Nothing
below was tested with an API key yet; see *Open questions*.

| Question | Result |
| --- | --- |
| Static API spec | None. Each workspace generates its own REST and GraphQL API from its data model; a custom object gets the same endpoints as `companies` |
| Record routes | Eight per object, identical for every object: `/rest/<plural>`, `/rest/<plural>/{id}`, `/rest/batch/<plural>`, `/rest/<plural>/duplicates`, `/rest/<plural>/merge`, `/rest/<plural>/groupBy`, `/rest/restore/<plural>` and `/rest/restore/<plural>/{id}` (`path.utils.ts`, `rest-api-core.controller.ts`) |
| Workspace OpenAPI | `GET /rest/open-api/core` returns the workspace's OpenAPI 3.1 document for any valid token (`PublicEndpointGuard`, `NoPermissionGuard`); without a token it returns a skeleton with no objects. Per object it has the paths above, a tag, and the schemas `<Singular>` (create, with `required`), `<Singular>ForUpdate` and `<Singular>ForResponse` (with relations). Select fields carry `enum` with the option values; relation fields appear as `<name>Id` (uuid) and as `$ref` to the target's `ForResponse` schema |
| Object metadata | `GET /rest/metadata/objects` needs the **Data Model** settings permission (`SettingsPermissionGuard(DATA_MODEL)`), even to read. A key with a restricted role cannot use it. Its list response comes in two shapes depending on the feature flag `IS_REST_METADATA_API_NEW_FORMAT_DIRECT` |
| Delete | `DELETE` deletes **permanently** unless `soft_delete=true`; the parser compares the query value with the string `'true'`, so a repeated parameter (an array) also means permanent |
| Bulk | `DELETE /rest/<plural>` requires a filter server-side. `PATCH /rest/<plural>` (update many) does **not**: without a filter it updates every record |
| Depth | `depth` accepts `0` or `1`; absent means `0` (the OpenAPI document claims a default of 1) |
| Paging | Cursor based: `limit` (capped at 200 server-side, default 60), `starting_after`, `ending_before`. List responses are `{"data":{"<plural>":[...]},"totalCount":N,"pageInfo":{"hasNextPage","startCursor","endCursor"}}` |
| Limits | Docs: 100 requests per minute, 60 records per batch call |
| Errors | Body `{"statusCode":N,"messages":[...],"error":"CODE"}` (seen live for 403). The REST exception filter answers unhandled exceptions with **400** |
| API key | A JWT: payload `sub` and `workspaceId` (the workspace ID), `type: "API_KEY"`, `jti` (the key's ID), `exp` (a key without expiry gets 100 years). A key can be assigned a role. Creating, updating or revoking a key needs a signed-in user's token (`RequireAccessTokenGuard`); an API key cannot manage keys |
| Our instance | `crm.wirdrei.digital`, `appVersion` v2.27.0 (from the public `/client-config`), password login only, single workspace. Latest release on 2026-09-25: v2.41.0 |

Sources: [APIs](https://docs.twenty.com/developers/extend/api),
[source at twenty/v2.27.0](https://github.com/twentyhq/twenty/tree/twenty/v2.27.0/packages/twenty-server/src/engine),
[twenty-sdk on npm](https://www.npmjs.com/package/twenty-sdk).

## Requirements

- **Any workspace, no rebuild.** Every object of the connected workspace, including custom objects,
  is a command, and a field added in Twenty's UI is usable at once. The command grammar is fixed in
  the binary; the workspace model is read at runtime.
- **API key auth only.** No browser, no OAuth.
- **Agent-first UX.** JSON in, JSON out, one line of JSON per failure, stable exit codes,
  self-describing help and a machine-readable catalog.
- **Nothing irreversible by accident.** Risk classes, `--force`, mandatory filters for bulk
  operations, soft delete by default, a read-only mode.
- **Public OSS.** README, SECURITY.md, `docs/agents.md`, CI, versioned multi-platform releases.

## What is different from the sibling CLIs

| Concern | bexio-cli, google-ads-cli | twenty-crm-cli |
| --- | --- | --- |
| Command source | A vendored spec, turned into an embedded manifest at build time | A fixed verb table in code, applied to the workspace model read at runtime from `/rest/open-api/core` and cached |
| Generator | `tools/fetchspec`, `tools/genmanifest` | none |
| Host | One fixed API host; others only with an `ALLOW_CUSTOM_BASE` opt-in | Any host; the stored key is bound to the base URL it was stored with |
| Credential | PAT, OAuth or a service-account key | API key (a JWT with an expiry and a role) |
| Risk classes | `read`, `write`, `delete`, `send` or `spend`, `admin` | `read`, `write`, `bulk`, `destroy`, `admin`, plus blocked commands |
| Paging | Offset or page tokens | Cursors (`starting_after`, `pageInfo.endCursor`) |

Everything else is carried over deliberately: Go and cobra, a single static binary
(`CGO_ENABLED=0`), stdout reserved for the response, one line of JSON on stderr per failure, exit
codes 0/1/2, `commands --json`, the `api` escape hatch, `--data` from an argument, a file or stdin
(with the UTF-8 BOM and UTF-16 handling), `--output`, `--all` with `--max-pages`, refusal to follow
redirects, secrets never from argv, the config file conventions and goreleaser releases.

## Architecture

```
twenty-crm-cli/
├── cmd/twentycrm/          # main: hands argv to internal/cli
├── internal/cli/           # command tree, flags, output, paging, config/auth/init/schema/api/commands
├── internal/api/           # HTTP client: base URL guard, headers, retries, errors, hints
├── internal/auth/          # API key parsing (JWT payload, no verification)
├── internal/config/        # config file, environment overlay, base URL normalisation and binding
├── internal/model/         # workspace model: extraction from the OpenAPI document, cache
├── internal/routes/        # the verb table, risk classes, local checks, api path classification
├── e2e/                    # smoke test against a fake Twenty; manual live test
└── docs/agents.md
```

Code is copied from `google-ads-cli` and adapted where the behaviour is the same: the API client
skeleton, the retry loop, error rendering, `--data` reading, `--output` handling, the config file,
the terminal prompt for `init`, and the CLI scaffolding for `commands`, `api` and `config`. The
repositories share no module. `internal/model` and `internal/routes` are new.

## Workspace model

The model is what the command tree, `--help`, `schema` and `commands --json` need to know about the
workspace. It is extracted from `GET /rest/open-api/core`, because that endpoint works with any valid
key, whatever its role; `/rest/metadata/objects` would need the Data Model permission.

**Extraction.** An object is a path `/<plural>` whose `get.operationId` is `findMany<Plural>` (first
letter capitalised). Its singular name comes from the same path's `post.operationId`
(`createOne<Singular>`), with the first letter lowercased. Its description comes from the tag named
after the plural. Its fields come from `components.schemas.<Singular>ForResponse.properties`; per
field the model keeps:

- `name`, `type` and `format` as the schema states them;
- `enum` (the option values of a select field; for a multi-select, the `enum` of `items`);
- `required`: listed in `components.schemas.<Singular>.required` (needed on create);
- `read_only`: present in `ForResponse` but absent from the create schema `<Singular>` (for example
  `id`, `createdAt`);
- `relation`: for a property that is a `$ref` to `<Target>ForResponse` (`many_to_one`) or an array
  of them (`one_to_many`), the target object's plural name;
- `description`.

The extraction fails with kind `server` and a message that names what it could not find when the
document has no `paths`, when an object's schemas are missing, or when no object at all can be read.
Commands that do not need the model (`version`, `config`, `auth`, `api`, `init` up to the fetch)
keep working.

**Cache.** The model is cached in `<user cache dir>/twentycrm/model-<id>.json`, where `<id>` is the
first 16 hex characters of the SHA-256 of `<base URL>|<workspace ID>`; file `0600`, directory
`0700`, written through a temp file and rename. It stores `fetched_at`, the base URL, the workspace
ID and the objects.

The command tree is built from the cache as it is; building it never touches the network. The
command's first word is its first argument that is not a flag (skipping the value of `--timeout` or
`--output` when given as a separate argument). When that word is not a built-in command, and the
cache is missing, older than 24 hours, or does not contain the word, the CLI refreshes the model
once, rebuilds the tree and resolves the command again. If that refresh fails, a stale cache that
contains the word is used and the command reports whatever its own API call reports; otherwise the
refresh error is reported. Every `schema` call reads live and rewrites the cache. Built-in commands,
`--help` without a command and completion never trigger a refresh.

An unreadable or corrupt cache is treated as missing. Without a configured key the tree has no
object commands, and an unknown command answers with the configuration hint.

## Commands

### Object commands

`twentycrm <object> <verb> [id] [flags]`, where `<object>` is the object's plural name in kebab case
(`companies`, `people`, `note-targets`, `workspace-members`, a custom `invoices`). The verb table is
the same for every object:

| Command | Request | Class | Notes |
| --- | --- | --- | --- |
| `list` | `GET /rest/<plural>` | `read` | `--filter`, `--order-by`, `--limit`, `--depth`, `--starting-after`, `--ending-before`, `--all`, `--max-pages` |
| `get <id>` | `GET /rest/<plural>/<id>` | `read` | `--depth` |
| `group-by` | `GET /rest/<plural>/groupBy` | `read` | `--group-by` (required), `--aggregate`, `--filter`, `--order-by`, `--limit`, `--view-id`, `--include-records-sample`, `--order-by-for-records` |
| `find-duplicates` | `POST /rest/<plural>/duplicates` | `read` | `--data` `{"ids":[...]}` or `{"data":[...]}`, `--depth` |
| `create` | `POST /rest/<plural>` | `write` | `--data` object, `--upsert`, `--depth` |
| `batch-create` | `POST /rest/batch/<plural>` | `write` | `--data` array of 1 to 60 objects, `--upsert`, `--depth` |
| `update <id>` | `PATCH /rest/<plural>/<id>` | `write` | `--data` object, `--depth` |
| `delete <id>` | `DELETE /rest/<plural>/<id>?soft_delete=true` | `write` | moves the record to the trash |
| `restore <id>` | `PATCH /rest/restore/<plural>/<id>` | `write` | `--depth` |
| `update-many` | `PATCH /rest/<plural>?filter=...` | `bulk` | `--filter` (required), `--data` object, `--depth` |
| `delete-many` | `DELETE /rest/<plural>?filter=...&soft_delete=true` | `bulk` | `--filter` (required) |
| `restore-many` | `PATCH /rest/restore/<plural>?filter=...` | `bulk` | `--filter` (required), `--depth` |
| `merge` | `PATCH /rest/<plural>/merge` | `bulk`; `read` with `--dry-run` | `--data` `{"ids":[...],"conflictPriorityIndex":N}`, `--dry-run`, `--depth` |
| `destroy <id>` | `DELETE /rest/<plural>/<id>?soft_delete=false` | `destroy` | permanent |
| `destroy-many` | `DELETE /rest/<plural>?filter=...&soft_delete=false` | `destroy` | `--filter` (required), permanent |

Flag and argument rules:

- `<id>` must be a UUID (`8-4-4-4-12` hex digits, either case); anything else is a usage error, so
  no argument can add a path segment.
- Query flags map to Twenty's names: `--order-by` to `order_by`, `--starting-after` to
  `starting_after`, `--ending-before` to `ending_before`, `--group-by` to `group_by`, `--view-id` to
  `view_id`, `--include-records-sample` to `include_records_sample`, `--order-by-for-records` to
  `order_by_for_records`. Values pass through as given; `--filter` and `--order-by` use Twenty's own
  syntax (for example `name[ilike]:"%acme%"`, `or(a[eq]:1,b[eq]:2)`, `createdAt[DescNullsLast]`).
- `--depth` accepts `0` or `1`. It is sent only when given.
- `--limit` accepts 1 to 200.
- `--upsert` sends `upsert=true`.
- `--dry-run` sets `"dryRun": true` in the merge body, overriding the body. Only the flag makes the
  call `read`; a `dryRun` field inside `--data` does not relax any gate.
- The body is sent as given apart from that one flag-owned field.

`--help` on an object's commands lists the object's fields from the cached model: name, type, the
`required` and `read_only` markers, the allowed values of select fields and relation targets. It
explains that `--filter` and `--order-by` use Twenty's syntax and shows two examples.

**Name collisions.** A built-in command wins over an object with the same kebab-case name. The
object is then listed in the catalog with `"shadowed_by": "<command>"` and stays reachable through
`api`. Built-in names: `version`, `commands`, `schema`, `api`, `config`, `auth`, `init`, `metadata`,
`help`, `completion`.

### Metadata commands

`twentycrm metadata <kind> <verb> [id]` for the kinds `objects`, `fields`, `views`, `view-fields`,
`view-filters`, `view-sorts`, `view-groups`, `view-filter-groups`, `page-layouts`,
`page-layout-tabs`, `page-layout-widgets`, `webhooks` and `api-keys`, at
`/rest/metadata/<camelCasePlural>` (`view-fields` is `viewFields`, `api-keys` is `apiKeys`):

| Verb | Request | Class |
| --- | --- | --- |
| `list` | `GET` with `--limit` (1 to 1000), `--starting-after`, `--ending-before`, `--all`, `--max-pages` | `read` |
| `get <id>` | `GET .../<id>` | `read` |
| `create` | `POST` with `--data` object | `admin` |
| `update <id>` | `PATCH .../<id>` with `--data` object | `admin` |
| `delete <id>` | `DELETE .../<id>` | `admin` |

`api-keys create`, `update` and `delete` are **blocked**: Twenty only lets a signed-in user manage
keys, a new key would appear once in the response and so in an agent's transcript, and revoking the
CLI's own key would cut it off. They stay in the tree and in the catalog with `"blocked": true` and
the reason, and are refused before any network I/O whatever the flags.

Deleting a field or an object removes its data for good; the `admin` class and its `--force` gate
cover that.

### Built-in commands

- `twentycrm schema` lists the workspace's objects (command name, plural and singular name,
  description, field count), read live. `twentycrm schema <object>` prints the object's fields as
  JSON, read live; `<object>` is the command name or the plural name (`note-targets` or
  `noteTargets`). Both rewrite the cache.
- `twentycrm commands --json` prints the catalog (see *Catalog*).
- `twentycrm api <METHOD> <path>` is the escape hatch (see *Guardrails*).
- `twentycrm config set|unset|path`, `twentycrm auth status`, `twentycrm init` (see
  *Authentication and configuration*).
- `twentycrm version` prints the CLI version, the commit and the Twenty version the release was
  tested against (`2.27`).
- Shell completion comes from cobra.

### Catalog

`commands --json` prints one JSON object, `schema_version` 1:

- `objects`: per object the command name, `name_plural`, `name_singular`, and `shadowed_by` when a
  built-in shadows it;
- `object_verbs`: per verb the HTTP method, the path template, the class, whether it takes an `id`,
  whether `--filter` is required, its flags, and the body limit (60 for `batch-create`);
- `metadata`: per kind and verb the method, path template, class, and `blocked` with its reason;
- `commands`: the built-in commands with a one-line description;
- `model_fetched_at`, and `model_missing: true` when no model could be loaded.

It lists the verb table once instead of repeating it per object, which keeps it small enough to read
in one call.

## CLI contract

- **Output:** the API response, untouched, on stdout, followed by a newline when it lacks one. An
  empty 2xx body prints nothing. `--output <file>` writes the body to a file instead; the file is
  opened before the request, so a bad path is a usage error with nothing sent. If writing still
  fails after a 2xx, the response goes to stdout and the call exits 1 with kind `output_failed`.
- **Bodies:** `--data '{...}'`, `--data @file.json` or `--data -`, capped at 20 MB, UTF-8 BOM
  stripped, UTF-16 refused with a message naming the fix.
- **`--all`:** on `list` and `metadata <kind> list`. It follows `pageInfo.endCursor` while
  `pageInfo.hasNextPage` is true, with 200 records per page for objects (1000 for metadata) unless
  `--limit` says otherwise, and prints one JSON array of the merged rows, the only transformation the
  CLI performs. Rows are read from `data.<plural>` when that is an array, else from `data` when that
  is an array; any other shape fails with kind `server` naming what it found. `--all` together with
  `--ending-before` is a usage error; `--starting-after` sets the starting point. `--max-pages`
  (default 100) caps it; hitting the cap with data remaining prints the partial array and exits 1
  with kind `incomplete`.
- **Global flags:** `--verbose` (requests to stderr, never a credential), `--timeout` (per attempt,
  default 30s), `--force`, `--output`.

## Authentication and configuration

**Credential.** A Twenty API key, sent as `Authorization: Bearer <key>`.

**Base URL.** `scheme://host[:port]`, normalised: lowercase scheme and host, no trailing slash, no
path, query, fragment or user info (anything else is a usage error that shows the accepted form).
HTTPS is required except for the loopback hosts `localhost`, `127.0.0.1` and `::1`. There is no
default; `init` suggests `https://api.twenty.com` for Twenty Cloud.

**Resolution.** Environment variables win over the config file:

- base URL: `TWENTY_BASE_URL`, else `base_url` from the file;
- key: `TWENTY_API_KEY`, else `api_key` from the file;
- read-only: `TWENTY_READ_ONLY=1` or `true`, or `read_only` from the file. The environment can only
  switch it on; `config unset read-only` switches off what the file set.

**Binding.** A key from the config file is sent only to the base URL stored with it. When the key
comes from the file and `TWENTY_BASE_URL` names another base URL, every API call is refused with a
usage error saying to set `TWENTY_API_KEY` as well or to unset `TWENTY_BASE_URL`. A key and a base
URL that both come from the environment are used as given: whoever sets both already holds the key.

**Key parsing.** A key has three dot-separated base64url segments; the middle one is decoded as JSON
without verifying the signature (the secret is on the server). `type` must be `API_KEY` and
`workspaceId` must be present; a user access token or anything else is refused with a message saying
what it is. `exp`, `jti` and `workspaceId` are read for `auth status` and the cache key. A key that
cannot be parsed is refused by `config set api-key` and `init`; from the environment it is sent as
given, and `auth status` reports that it could not be read.

**Config file.** `<user config dir>/twentycrm/config.json`, `0600` inside a `0700` directory, written
through a temp file and rename. `twentycrm config path` prints the location.

- `config set base-url <url>` normalises and stores the URL. When a key is stored for a different
  base URL it refuses and says to run `config unset api-key` first; the same URL is a no-op.
- `config set api-key` reads the key from stdin only (never argv), requires a stored base URL,
  parses it as above and stores it. It prints the workspace ID, the key ID and the expiry.
- `config set read-only <true|false>`.
- `config unset base-url|api-key|read-only`. `unset base-url` with a stored key refuses and says to
  unset the key first. `unset api-key` says that the key stays valid until it is revoked in Twenty
  (Settings, APIs & Webhooks).
- An environment variable name or an unknown key is a usage error that names the valid keys.
- After a `set`, a note on stderr says when an environment variable overrides the saved value.

**`twentycrm auth status`.** Offline. One line of JSON: `mode` (`api_key` or `none`), `source`
(`env` or `config`), `base_url`, `base_url_source`, `workspace_id`, `key_id`, `expires_at`
(RFC 3339), `expires_in_days`, `expires_soon` (fewer than 14 days left), `expired`, `read_only`, and
`missing` plus `hint` when the configuration cannot make a call. Never the key.

**`twentycrm init`.** Needs a terminal and refuses without one. It asks for the base URL, then for
the key with hidden input, parses the key, and verifies both with `GET /rest/open-api/core`. It then
tries `GET /rest/companies?limit=1&depth=0` for the company count and skips the count when that
fails. It shows the workspace ID, the number of objects, the company count and the key's expiry,
asks whether to switch on read-only mode (default no), then saves the base URL, the key and the
answer and writes the model cache. When the file already holds a key for another base URL, it asks
before replacing it. When verification fails it prints the error with its hint and saves nothing;
the non-interactive `config set` path stays available.

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
which makes it easier to undo than an `update`, which is not gated either.

### Local checks

Before anything is sent, in this order, each a usage error with exit 2:

1. Argument and body shape: a UUID for `<id>`; `--depth` 0 or 1; `--limit` in range; `--data` valid
   JSON of the shape the verb takes (an object; an array of 1 to 60 objects for `batch-create`; for
   `merge` an object with an `ids` array of at least two strings; for `find-duplicates` an object
   with `ids` or `data`); `--data` required where the verb has a body.
2. A required `--filter` that is missing or blank.
3. A blocked command.
4. Read-only mode for anything but `read`.
5. `--force` for `bulk`, `destroy` and `admin`. The message names the command, its class and why it
   needs the flag.

`--force` is a per-call flag with no environment or config equivalent. The CLI never splits a batch:
the parts would no longer succeed or fail together.

### The `api` escape hatch

`twentycrm api <METHOD> <path>` with `--data`, `--query k=v` (repeatable) and `--header k:v`
(repeatable), for a path relative to the base URL. Methods: `GET`, `POST`, `PATCH`, `PUT`, `DELETE`.

- The path must start with `rest/` (a leading `/` is accepted). GraphQL (`graphql`, `metadata`) is
  refused in v1.
- Every path segment follows the positional rules: no `%`, whitespace, `\`, empty, `.` or `..`
  segment, so no spelling of a path dodges the match. A query string inside the path is refused;
  queries come from `--query`.
- `Authorization` cannot be set, nor the method-override headers (`X-HTTP-Method-Override`,
  `X-HTTP-Method`, `X-Method-Override`), compared case-insensitively with `_` and `-` as the same
  character. A body on `GET` or `DELETE` is refused.
- `--query soft_delete` and `--query filter` may each appear at most once.

The path takes a class from the route grammar, independent of the model. Rows are tried from top to
bottom and the first match wins; `<o>` and `<id>` each stand for exactly one segment:

| Path | Method | Class |
| --- | --- | --- |
| `rest/apiKeys...`, `rest/metadata/apiKeys...` | `GET` | `read` |
| same | other | blocked |
| `rest/webhooks...`, `rest/metadata/...` | `GET` | `read` |
| same | other | `admin` |
| `rest/open-api/...` | `GET` | `read` |
| `rest/batch/<o>` | `POST` | `write` |
| `rest/restore/<o>/<id>` | `PATCH` | `write` |
| `rest/restore/<o>` | `PATCH` | `bulk`, filter required |
| `rest/<o>/duplicates` | `POST` | `read` |
| `rest/<o>/merge` | `PATCH` | `bulk` |
| `rest/<o>/groupBy` | `GET` | `read` |
| `rest/<o>/<id>` | `GET` | `read` |
| `rest/<o>/<id>` | `PATCH`, `PUT` | `write` |
| `rest/<o>/<id>` | `DELETE` | `write` when `soft_delete` is exactly `true`, else `destroy` |
| `rest/<o>` | `GET` | `read` |
| `rest/<o>` | `POST` | `write` |
| `rest/<o>` | `PATCH`, `PUT` | `bulk`, filter required |
| `rest/<o>` | `DELETE` | `bulk` when `soft_delete` is exactly `true`, else `destroy`; filter required |
| anything else | `GET` | `read` |
| anything else | other | `admin` |

The gates are the same as for commands. The API client itself refuses a non-GET request that carries
no class, so no call site can send a change past the guard.

### Read-only mode

`TWENTY_READ_ONLY=1` or `config set read-only true` allows `read`-class calls only, including through
`api`.

### What the CLI cannot do

It cannot stop a caller with shell access from passing `--force`. It makes that decision explicit and
visible. The hard boundary is the role assigned to the API key in Twenty (Settings, Roles): it
decides which objects the key can read or change and whether it may touch the data model. The README
recommends a dedicated role per ICM with exactly the objects it needs. Wiring the CLI into an
operator's own workspace (agent rules, where the key lives) happens in that workspace and is not part
of this repository.

## Errors, exit codes, retries

- **Exit codes:** `0` success, `1` API or network error, `2` usage error (including every guardrail
  refusal).
- **Error line** on stderr: `{"kind","error","status","details"}`. `status` is the HTTP status when a
  response exists and is omitted otherwise. `details` is the parsed error body, a string for a
  non-JSON body, `null` when empty.
- **Kinds:** `auth`, `forbidden`, `not_found`, `validation`, `conflict`, `rate_limited`, `server`,
  `transport`, `outcome_unknown`, `incomplete`, `usage`, `output_failed`.
- **Mapping:** 400 and 422 to `validation`, 401 to `auth`, 403 to `forbidden`, 404 and 410 to
  `not_found`, 409 to `conflict`, 429 to `rate_limited`, 5xx to `server`, 3xx to `validation`
  (redirects are refused and the message names the `Location`). The README notes that Twenty answers
  unhandled server errors with 400.
- **Message:** `<METHOD> <path>: HTTP <status>: <first entry of messages>`, with `(and N more)` when
  there are more, then a hint when one applies:
  - 401 whose message mentions expiry: the key has expired; create a new one in Settings, APIs &
    Webhooks and run `twentycrm config set api-key`;
  - other 401: the key is invalid or revoked; `twentycrm auth status` shows which key is in use;
  - 403 on `rest/metadata/objects` or `rest/metadata/fields`: this needs the Data Model permission;
    `twentycrm schema` works without it;
  - other 403: the key's role does not allow this on this object (Settings, Roles);
  - 400 on an object command whose message contains the word `field` (any case): run
    `twentycrm schema <object>` for the field names;
  - 429: Twenty allows 100 requests per minute.
- **Retries:** 429 is retried for every call with exponential backoff (from 1 second, doubling) and
  jitter, honouring `Retry-After` when present, within a total budget of 60 seconds; when the next
  wait would exceed it, the call fails with `rate_limited`. Transport errors and 5xx are retried only
  for `read`-class calls (which includes `find-duplicates` and `merge --dry-run`), up to three
  attempts. A dial or DNS failure never reached Twenty and is retried like a read. Any other call
  that fails in flight is never replayed and ends as `outcome_unknown`, telling the caller to check
  the state before retrying.
- **Deadlines:** `--timeout` per attempt, default 30 seconds. Ctrl-C and SIGTERM cancel the request
  and any retry wait.

## Testing

- **Model**, against fixture OpenAPI documents in `internal/model/testdata/`: a hand-built document
  in the exact shape Twenty v2.27 generates (from `open-api.service.ts`, `path.utils.ts`,
  `components.utils.ts` and `convert-object-metadata-to-schema-properties.util.ts` at
  `twenty/v2.27.0`) with `companies`, `people`, `noteTargets` and a custom object, covering select
  and multi-select enums, required and read-only fields, both relation kinds and composite fields;
  the skeleton an anonymous caller gets; and malformed documents. Cache: missing, fresh, stale,
  corrupt, keyed by base URL and workspace. After the live check, a sanitised recording (custom
  fields replaced by neutral ones, since the repository is public) joins the fixtures.
- **Routes**, table-driven: every verb's method, path, query and class; every local check; every row
  of the `api` classification table, including encoded and dotted segments, a repeated
  `soft_delete`, `soft_delete=TRUE`, and paths with a query string.
- **Auth and config:** key parsing (valid, user token, garbage, missing `exp`), base URL
  normalisation, the binding matrix (key and URL from env or file), `config set/unset/path`
  semantics and file permissions, `auth status` output, `init` refusing without a terminal and
  running against a fake prompter.
- **API client**, against a fake Twenty server: the bearer header, error mapping with hints, retries
  (429 with and without `Retry-After`, an exhausted budget, 5xx on reads, no replay of writes),
  refused redirects, the HTTPS rule, `--all` with `--max-pages` and both metadata list shapes.
- **CLI end to end:** the built binary against a fake Twenty that serves the fixture OpenAPI document
  and implements enough of the record routes: the tree built from the model, a refresh on an unknown
  object, `commands --json`, `schema`, create through destroy, the gates.
- **Live, manual, not in CI** (`TWENTY_LIVE=1` with a configured key):
  1. `auth status` shows the workspace and an expiry;
  2. `schema companies` lists the select values of a known select field;
  3. `companies list --all --depth 0` returns as many rows as `totalCount` says;
  4. with a key whose role lacks the Data Model permission, `schema` still works and
     `metadata objects list` fails with `forbidden` and the hint.

  Writes run only with `TWENTY_LIVE_WRITE=1`: a company named `twentycrm-live-<unix time>` is
  created, read, updated, deleted, listed (absent), restored, and destroyed with `--force`; a final
  `get` returns `not_found`. Each step checks its result.

## CI and release

- GitHub Actions on Ubuntu, macOS and Windows: `go vet` and `go test ./...` (`make check`), plus a
  cross-compile check.
- goreleaser on tag: darwin amd64/arm64, linux amd64/arm64, windows amd64, `checksums.txt`, GitHub
  Release. `go install github.com/wir-drei-digital/twenty-crm-cli/cmd/twentycrm@latest` works too.
- SemVer. The public API is the command grammar, flags, exit codes, the stderr error schema and the
  `commands --json` schema. Additive optional fields are allowed under the same `schema_version`.
- First release `v0.1.0` after the live check passes.
- **Twenty versions:** the README names the version each release was tested against. The route
  grammar has been stable, but a Twenty upgrade on the operator's side should be followed by the
  read-only live check.

## Documentation

- **README:** install, operator setup, authentication, the command grammar, the workspace model and
  its cache, guardrails, the error contract, the tested Twenty version, and that the project is not
  affiliated with Twenty.
- **SECURITY.md:** what an API key is (a JWT bound to one workspace, with an expiry chosen at
  creation and a role), where it lives (config file versus `TWENTY_API_KEY`), the base URL binding,
  revocation in Twenty's settings, and what the CLI never does (follow redirects, log credentials,
  take secrets from argv, manage keys).
- **docs/agents.md:** the paste-in agent reference: `schema` before writing, filter and order syntax
  with examples, `--depth 0` to keep responses small, batches of at most 60, linking a note or task
  to a record through `note-targets` or `task-targets`, `delete` versus `destroy`, never `--force`
  without a person's go-ahead, error kinds and hints.

## Operator setup (once per workspace)

1. In Twenty, Settings, Roles: create a role for the agent with the object permissions it needs and
   no settings permissions, unless it should change the data model (Data Model) or webhooks (API
   Keys & Webhooks).
2. Settings, APIs & Webhooks: create a key with that role and an expiry. Copy it; it is shown once.
3. On each machine: `twentycrm init`, or `config set base-url` and `config set api-key < key.txt`, or
   the environment variables.
4. Note the expiry date; `twentycrm auth status` shows it and flags it 14 days ahead.

## Out of scope for v1

- GraphQL, and with it the batch upsert by plural names; REST covers upsert and batch creation.
- OAuth (authorization code or client credentials).
- Receiving webhooks.
- Several workspaces as named profiles; the environment variables cover a second one.
- `POST /rest/dashboards/{id}/duplicate`, workflow runs and file uploads for attachments; `api`
  reaches the first as an `admin`-class call.
- Human-readable table output, OS keychain storage, a Homebrew tap, artifact signing.

## Open questions

These need a key and are settled by the live check:

- Whether Twenty sends `Retry-After` on 429, and whether the 100-per-minute limit is per key or per
  workspace.
- Whether the server enforces the 60-record batch limit, and with which error.
- Whether `/rest/open-api/core` lists objects the key's role cannot read; if so, their commands exist
  and answer `forbidden`.
- Which metadata list shape our instance returns.
