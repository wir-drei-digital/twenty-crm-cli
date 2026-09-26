# twentycrm: agent reference

Paste the block below into your agent's instructions. It is the whole contract.

```markdown
# twentycrm (Twenty CRM CLI): agent reference

Setup is a person's job. Never run `twentycrm init` or `twentycrm config set`, and never pass
`--force` unless a person told you to, in this conversation, for this exact call.

Before you write
- `twentycrm schema <object>`: the object's fields as JSON, read live. `enum` lists the only
  values a select field accepts; `required` fields are needed on create; `read_only` fields are
  set by Twenty; `subfields` are the parts of a composite field (emails.primaryEmail); a
  many_to_one relation is set through `<name>Id`.
- `twentycrm schema` lists the objects; `twentycrm commands --json` is the full catalog.

Commands: `twentycrm <object> <verb> [id] [flags]`, object = plural name in kebab case
(companies, people, tasks, notes, note-targets, task-targets, or a custom object).
- Read: `list` (--filter, --order-by, --limit up to 200, --depth 0|1, --all), `get <id>`,
  `group-by --group-by '[{"city":true}]'`, `find-duplicates --data '{"ids":["<id>"]}'`.
- Write: `create --data '{...}'`, `batch-create --data '[...]'` (at most 60 records),
  `update <id> --data '{...}'`, `delete <id>` (to the trash), `restore <id>` (answers with
  `data.restore<Plural>`, an array: the restored record, or empty when nothing matched).
- Needs --force: `update-many`, `delete-many`, `restore-many` (all need --filter), `merge`,
  `destroy <id>` and `destroy-many` (permanent), and `metadata ... create/update/delete`.
- `merge --dry-run` previews a merge without --force.
- `--data` takes a JSON literal, `@file.json` or `-` for stdin. Build bulk payloads with a
  script from the source file; never type records from memory.

Filters: field[comparator]:value, commas mean "and", or(...) and not(...) combine.
Comparators: eq neq in containsAny is gt gte lt lte startsWith endsWith like ilike.
The field names below are examples; `twentycrm schema <object>` is the source for the real ones.
  --filter 'name[ilike]:"%acme%"'                          (companies)
  --filter 'or(city[eq]:Bern,jobTitle[ilike]:"%CEO%")'     (people)
  --filter 'emails.primaryEmail[eq]:ana@example.com'       (people)
Order: --order-by 'createdAt[DescNullsLast],name'. Use --depth 0 unless you need relations.

Linking: a note or task is linked to a record through note-targets or task-targets. The link
field is named after the target (targetCompanyId, targetPersonId, targetOpportunityId in
Twenty 2.27); `twentycrm schema note-targets` lists them:
  twentycrm notes create --data '{"title":"Call","bodyV2":{"markdown":"..."}}'
  twentycrm note-targets create --data '{"noteId":"<note id>","targetCompanyId":"<company id>"}'

Output and errors
- stdout is the API response, untouched; `--all` prints one JSON array of the records.
- Exit 0 success, 1 API or network error, 2 usage error or refused by a guard (nothing sent).
- stderr is one JSON line: {"kind","error","status","details"}. Kinds: auth, forbidden,
  not_found, validation, conflict, rate_limited, server, transport, outcome_unknown,
  incomplete, usage, output_failed.
- outcome_unknown: a write may or may not have happened; read before you retry.
- incomplete: --all stopped at --max-pages; the output is partial. When --all fails after the
  first page, stdout also holds the rows collected so far; check the exit code before using them.
- The error message ends with a hint when one applies (expired key, missing permission, unknown
  field). Twenty allows 100 requests per minute; the CLI waits and retries on 429 by itself.
```

## Notes for the person wiring this up

- Give the agent a key whose role has exactly the objects it needs (Twenty: Settings, Roles).
  The role is the real boundary; `--force` only stops mistakes.
- `twentycrm auth status` shows the key's expiry and flags it 14 days ahead. Put the date in a
  calendar.
- `TWENTY_READ_ONLY=1` makes a runner read-only whatever its key allows.
