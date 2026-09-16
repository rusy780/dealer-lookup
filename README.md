# dealer-lookup

Small Go CLI that prices a dealership inventory CSV using a local OpenClaw model.

It reads your inventory CSV, skips anything it has already priced, sends only the
unique vehicles to OpenClaw in batches, and writes enriched CSVs plus a run manifest.

**What it does per run**

1. Reads your inventory CSV (`INPUT_CSV`)
2. Copies the original into a timestamped run folder
3. Skips rows already in `data/cache.json` (exact VIN+price+miles, or a similar-vehicle group)
4. Groups the remaining rows and sends **one representative per group** to OpenClaw
5. Fans each group's answer back out to every member of that group
6. Writes `enriched_inventory.csv`, `vehicle_details.csv`, and `manifest.json`

No database. No cloud API key. Inference runs through your local OpenClaw install.

---

## Requirements

- **Go 1.23+** (`go.mod` targets 1.23.2)
- **OpenClaw CLI** on your `PATH`, already authenticated with at least one provider

Check both:

```bash
go version
openclaw --version
```

---

## Setup

### 1. Confirm OpenClaw can answer in the format this tool needs

This is the exact call the program makes. Run it first — if this fails, nothing else will work:

```bash
openclaw infer model run --json --prompt 'Return {"r":[["TEST",1,2,1,75]]}'
```

You want `"ok": true` and an `outputs[0].text` containing the JSON:

```json
{
  "ok": true,
  "capability": "model.run",
  "transport": "local",
  "provider": "openai",
  "model": "gpt-5.6-luna",
  "outputs": [
    { "text": "{\"r\":[[\"TEST\",1,2,1,75]]}", "mediaUrl": null }
  ]
}
```

### 2. Put your inventory at the input path

Default is `data/input/inventory.csv`. Required headers (case-insensitive, any order):

```csv
VIN,Year,Make,Model,Trim,Price,Miles
```

Extra columns are ignored. `Price` and `Miles` tolerate `$` and `,` — `$18,500` parses fine.
A row with no VIN is never sent for lookup.

### 3. Run it

```bash
go run .
```

That is the whole setup. The first run creates `data/cache.json` and a run folder under `data/output/`.

---

## Choosing the model

By default OpenClaw picks your selected provider/model. To pin one, set `OPENCLAW_MODEL`,
which is passed straight through to `openclaw infer model run --model`:

```bash
export OPENCLAW_MODEL="anthropic/claude-haiku-4-5"
go run .
```

List what is available to you:

```bash
openclaw infer model list        # every known model id
openclaw infer model providers   # providers, with "configured": true/false
```

Use a small, fast, cheap model. The task is narrow — return five numbers per row — so a
large reasoning model mostly costs you time.

---

## Transports: CLI vs HTTP

`OPENCLAW_BASE_URL` selects how the tool talks to the model.

| Value | Transport |
|---|---|
| `cli` (default), `openclaw`, or empty | Runs the `openclaw` binary as a subprocess |
| any `http://` or `https://` URL | POSTs an OpenAI-style `/chat/completions` request to that URL |

`OPENCLAW_URL` and `OPENCLAW_API_URL` are accepted as aliases for `OPENCLAW_BASE_URL`.

Pointing at a separate OpenAI-compatible server (Ollama, vLLM, LM Studio, …):

```bash
export OPENCLAW_BASE_URL="http://127.0.0.1:11434/v1/chat/completions"
export OPENCLAW_MODEL="qwen2.5:7b"
go run .
```

PowerShell:

```powershell
$env:OPENCLAW_BASE_URL="http://127.0.0.1:11434/v1/chat/completions"
$env:OPENCLAW_MODEL="qwen2.5:7b"
go run .
```

---

## Running without OpenClaw

Uses a cheap local estimator (price minus 2%, or 4% over 100k miles). Good for testing the
pipeline end to end without spending any inference:

```bash
OPENCLAW_ENABLED=false go run .
```

Those rows are marked `source=local-cheap-fallback` in the output so you can tell them apart.

---

## Configuration

All settings are environment variables. Defaults shown.

### Paths

```bash
INPUT_CSV=data/input/inventory.csv
OUTPUT_DIR=data/output
CACHE_PATH=data/cache.json
CACHE_WARMUP_PATHS=
```

### Lookup behavior

```bash
MAX_LOOKUPS_PER_RUN=0     # 0 = no cap. Caps GROUPS sent per run, not rows.
BATCH_SIZE=250            # rows per request
CACHE_TTL_DAYS=30         # cached results older than this are re-looked-up
```

### OpenClaw

```bash
OPENCLAW_ENABLED=true
OPENCLAW_BASE_URL=cli     # "cli" | "openclaw" | an http(s) URL
OPENCLAW_MODEL=           # empty = OpenClaw's own default
OPENCLAW_CONCURRENCY=2    # batches in flight at once
OPENCLAW_TIMEOUT_SECONDS=120
OPENCLAW_MAX_TOKENS=4000  # HTTP transport only
OPENCLAW_TEMPERATURE=0.1  # HTTP transport only
```

### Cost + failure handling

```bash
TOKEN_PRICE_PER_MILLION=0.05
USE_MOCK_IF_DOWN=false    # false = a failed lookup aborts the run
```

`USE_MOCK_IF_DOWN=false` means OpenClaw is **required**: if it stays down or keeps returning
unparseable JSON, the run exits with an error rather than quietly filling your inventory with
estimated prices. Set it to `true` only when you would rather finish the file than be correct.

Note that `MAX_LOOKUPS_PER_RUN` caps the number of **groups** queued, not the number of rows
processed — one group can cover many rows.

---

## How it keeps cost down

The expensive thing is tokens, so the tool avoids sending rows at all:

- **Exact cache** — same VIN + price + miles is never looked up twice
- **Group cache** — vehicles are bucketed by year / make / model / trim / $1,000 price bucket /
  10,000 mile bucket, and only one representative per bucket is sent
- **Compact wire format** — sends `id,Year,Make,Model,Trim,Price,Miles` CSV rows, and asks for
  only `[id, low, high, recommended, confidence]` back
- **Local text** — summaries and flags are generated locally, never spending output tokens
- **Optional hard cap** — `MAX_LOOKUPS_PER_RUN`

Two 2019 Honda Accord Sports at $18,200 and $18,900 with 64k and 67k miles share one lookup.

`manifest.json` records estimated tokens, estimated cost, and a projected cost per 1M rows using
`TOKEN_PRICE_PER_MILLION`. It is an approximation — real tokenization depends on your model.

### Reusing results from earlier runs

`CACHE_WARMUP_PATHS` imports prior `vehicle_details.csv` files into the cache before filtering.
Accepts a path, a glob, or a comma-separated list:

```bash
CACHE_WARMUP_PATHS='data/output/*/vehicle_details.csv' go run .
```

---

## Output

Each run creates `data/output/<UTC timestamp>/`:

```txt
data/output/20260521_181803/
├── original_inventory.csv    # verbatim copy of the input
├── enriched_inventory.csv    # every input row + lookup columns
├── vehicle_details.csv       # only rows that have a result
└── manifest.json             # counts, token + cost estimates, paths
```

`enriched_inventory.csv` is your input columns plus:

`lookup_group`, `lookup_needed`, `lookup_used`, `lookup_source`, `lookup_cached`,
`suggested_price_low`, `suggested_price_high`, `recommended_price`, `lookup_confidence`,
`dealer_summary`, `lookup_flags`

The `source` / `lookup_source` column tells you where a price came from:

| Value | Meaning |
|---|---|
| `local-openclaw` | Priced by the model this run |
| `local-cheap-fallback` | Local estimator (disabled, or down with `USE_MOCK_IF_DOWN=true`) |
| `cache-warmup` | Imported via `CACHE_WARMUP_PATHS` |

Flags currently emitted are `PRICE_REVIEW` and `HIGH_MILES` (100k+ miles), joined with `|`.

---

## Troubleshooting

**`could not parse local OpenClaw JSON: ...`**

The model returned prose, or OpenClaw's console banners got mixed into its output. The tool reads
the subprocess's stdout *and* stderr together, so startup notices can land in the parse buffer.
Silence them:

```bash
OPENCLAW_LOG_LEVEL=silent go run .
```

If it persists, the model is ignoring the format. Pin a more reliable one via `OPENCLAW_MODEL`,
or lower `BATCH_SIZE` — long batches are likelier to get a truncated or chatty reply.

**`openclaw CLI failed: ... executable file not found`**

`openclaw` is not on the `PATH` of the shell running `go run .`. Check with `command -v openclaw`.

**`openclaw HTTP request failed for ...`**

`OPENCLAW_BASE_URL` is set to a URL that is not answering. To use the local CLI instead, set it
back to `cli`.

**`openclaw CLI timed out after 2m0s`**

Raise `OPENCLAW_TIMEOUT_SECONDS`, lower `BATCH_SIZE`, or both. A failed batch is automatically
retried as two smaller halves before it gives up.

**`missing required header: vin`**

Your CSV is missing one of `VIN,Year,Make,Model,Trim,Price,Miles`. Header matching ignores case
and surrounding spaces, but every one of the seven must be present.

**`error obtaining VCS status: exit status 128`**

Only affects `go build`, not `go run`. It means git refuses the repo directory (usually mismatched
ownership). Either:

```bash
go build -buildvcs=false ./...
```

or fix the ownership complaint:

```bash
git config --global --add safe.directory /root/zux/dealer-lookup
```

---

## Development

```bash
go test ./...                  # tests live in lookup/
go vet -buildvcs=false ./...
go build -buildvcs=false -o dealer-lookup .
```

Layout:

| Path | Role |
|---|---|
| `main.go` | Run orchestration |
| `config/` | Environment configuration |
| `importcsv/` | Input CSV parsing |
| `lookup/` | Cache, grouping, and the OpenClaw client |
| `models/` | `Vehicle` / `LookupResult`, group-key bucketing |
| `exportcsv/` | Output CSVs and manifest |
| `storage/` | Run folders |

### A note on `data/`

`data/` holds the input CSV, every run folder, and the cache, and it grows quickly — a full
inventory run leaves a 60MB+ `data/cache.json`. There is no `.gitignore` in this repo, so add one
before committing:

```gitignore
data/cache.json
data/output/
```
