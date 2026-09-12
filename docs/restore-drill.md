# Database restore drill

A backup you have never restored is not a backup. You do not know the file is valid,
how long it takes, what breaks, or what you lose. This is the rehearsal, run against a
scratch database so production is never touched.

Run it **quarterly**, and after any change to the database or the hosting plan.

Last run: **12 September 2026** — passed. Numbers and findings at the bottom.

---

## 0. Prerequisites (one time)

Production is **PostgreSQL 17.6** (Supabase). `pg_dump` refuses to dump from a server
newer than itself, so the client tools must be 17 or later:

```bash
brew install postgresql@17
/opt/homebrew/opt/postgresql@17/bin/pg_dump --version   # must say 17.x
```

Every command below assumes that path. Add it to your shell if you prefer:

```bash
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"
export LC_ALL="en_US.UTF-8"
```

`LC_ALL` is not optional. Without it PostgreSQL 17 on macOS refuses to start:
`FATAL: postmaster became multithreaded during startup`. It costs twenty minutes to
work out during a real incident.

**Use the direct connection, not the transaction pooler.** The pooler on port 6543
multiplexes statements across connections and will corrupt a dump. Supabase shows the
direct string under *Project Settings → Database → Connection string → URI*. The
`DB_HOST` in `.env` is the pooler host and is fine for the app, not for `pg_dump`.

---

## 1. Take a dump (5 minutes)

```bash
cd ~/Desktop/invo-server
set -a && . ./.env && set +a          # loads DB_* without printing them
export PGPASSWORD="$DB_PASSWORD"
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"

STAMP=$(date +%Y%m%d-%H%M)
pg_dump \
  "host=$DB_HOST port=$DB_PORT user=$DB_USER dbname=$DB_NAME sslmode=require" \
  --format=custom --no-owner --no-privileges \
  --file="$HOME/backups/invo-$STAMP.dump"

ls -lh "$HOME/backups/invo-$STAMP.dump"
```

`--format=custom` so `pg_restore` can be selective later. `--no-owner --no-privileges`
because Supabase's roles do not exist on your machine and would make the restore noisy.

**A dump that errors is a failed drill.** Stop and fix it — that is the finding.

Dumping through the pooler host in `.env` does work, and took 26 seconds for a 12 MB
database. The direct connection is still preferable for anything larger.

---

## 2. Restore into a scratch database (5 minutes)

Never into production. This creates a throwaway local cluster, restores into it, and
leaves production untouched.

```bash
export PATH="/opt/homebrew/opt/postgresql@17/bin:$PATH"
export LC_ALL="en_US.UTF-8"
rm -rf /tmp/restoredrill
initdb -D /tmp/restoredrill -U postgres --auth=trust >/dev/null
pg_ctl -D /tmp/restoredrill -o "-p 55500 -h 127.0.0.1 -k /tmp" -l /tmp/restoredrill.log start
sleep 3
createdb -h 127.0.0.1 -p 55500 -U postgres invo_restored

time pg_restore \
  --host=127.0.0.1 --port=55500 --username=postgres \
  --dbname=invo_restored --no-owner --no-privileges \
  "$HOME/backups/invo-$STAMP.dump"
```

**Write down how long that took.** That number is your recovery time, and it is the
only honest answer to "how long would we be down".

### Errors you can ignore

```
pg_restore: error: could not execute query: ERROR:  relation "vault.secrets" does not exist
pg_restore: warning: errors ignored on restore: 3
```

Expected. Supabase keeps its own `vault` and `auth` schemas alongside yours, and they
have no meaning on a plain PostgreSQL server. **Only errors naming your own tables
matter.** Check the count afterwards rather than trusting a clean-looking log.

---

## 3. Verify it is actually usable (5 minutes)

A restore that completes is not a restore that worked. Check three things.

**Every table came back, with the right number of rows:**

Use a shell *function*, not a variable — zsh will not word-split a string into
arguments and you will get `command not found`:

```bash
q() { psql -h 127.0.0.1 -p 55500 -U postgres -d invo_restored -tAc "$1"; }

echo "public tables: $(q "select count(*) from information_schema.tables where table_schema='public'")"

for t in users companies clients items invoices invoice_items payments \
         payment_allocations credit_notes estimates ledger_entries \
         stock_movements expensess categories; do
  printf "%-22s %s\n" "$t" "$(q "select count(*) from $t")"
done
```

Compare against production (same loop, production connection). Any table that differs
is a finding.

**The schema is at the right migration:**

```bash
$PSQL "select version, dirty from schema_migrations"
```

`dirty = f`, and `version` must match `ls migrations | tail -1`.

**The money still reconciles** — this is the one that catches a subtly broken restore:

```bash
$PSQL "select i.invoice_number,
              i.total,
              round(i.subtotal - i.discount + i.tax, 2) as recomputed
       from invoices i
       where i.status <> 'draft'
         and round(i.subtotal - i.discount + i.tax, 2) <> i.total"
```

Zero rows is a pass. Any row means the restored data disagrees with itself.

---

## 4. Point the app at the restored copy (5 minutes)

The last step nobody rehearses, and the one that matters at 2am: can the application
actually run on it?

```bash
cd ~/Desktop/invo-server
ENVIRONMENT=development \
DB_HOST=127.0.0.1 DB_PORT=55500 DB_USER=postgres DB_PASSWORD=x \
DB_NAME=invo_restored DB_SSLMODE=disable \
JWT_SECRET=drill PORT=55501 RUN_MIGRATIONS=false SENTRY_DSN= \
  go run ./cmd/server
```

`ENVIRONMENT=development` is required. `go run` loads `.env`, which sets
`ENVIRONMENT=production`, and the server then refuses to start with
`DB_SSLMODE=disable is not allowed in production` — the guard doing its job against a
local database that has no TLS. `SENTRY_DSN=` keeps drill noise out of the dashboard.

In another terminal:

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:55501/health   # expect 200
```

Then sign in through the web app against it and open one invoice. If that works, the
backup is real.

---

## 5. Tear down

```bash
pg_ctl -D /tmp/restoredrill stop -m fast
rm -rf /tmp/restoredrill /tmp/restoredrill.log
```

Keep the dump file. Store at least one off-machine — a dump that only exists on the
laptop that also holds the credentials is one theft away from nothing.

---

## Record the result

| Date | Dump size | Dump time | Restore time | Findings |
|------|-----------|-----------|--------------|----------|
| 2026-09-12 | 313 KB (12 MB db) | 26s | 1.4s | Passed. Three environment problems found and fixed, below. |

**What the first drill found** — none of it would have been known during an incident:

1. `pg_dump` was version 15 against a 17.6 server and **refused to run**. There was no
   working way to take a backup at all.
2. PostgreSQL 17 would not start on macOS without `LC_ALL`.
3. The app would not boot against the restored copy, because `.env` sets
   `ENVIRONMENT=production` and the production guard rejects an unencrypted
   connection.

**What it verified:** 26 tables restored; row counts identical to production across
all 14 checked tables (182 items, 18 invoices, 20 ledger entries, 5 payments);
`schema_migrations` at 52, not dirty, matching the newest migration file; zero invoices
where `subtotal - discount + tax` disagreed with the stored total; no orphaned payment
allocations or invoice items; and the real server booted against it and answered
`/health` with 200 and a login attempt with a correct 401.

---

## If this is not a drill

When production is actually gone, the order changes:

1. **Stop writes first.** Suspend the Render service so the app cannot write into a
   half-restored database and make the damage worse.
2. **Prefer Supabase's own restore.** *Project Settings → Database → Backups* restores
   a point in time without moving any data yourself, and loses less than a nightly
   dump. Use your dump only if their backup is unusable — that is what it is for.
3. **If restoring from the dump**, create a fresh Supabase project, restore into it
   (step 2 above, against the new project's direct connection), then change `DB_HOST`,
   `DB_USER`, `DB_PASSWORD` and `DB_NAME` on Render and redeploy.
4. **Leave `RUN_MIGRATIONS` alone.** The dump already carries the schema at its
   migration version; letting migrations run against a restored database is how a
   recovery turns into a second incident.
5. **Then verify with step 3**, before telling anyone it is over.
