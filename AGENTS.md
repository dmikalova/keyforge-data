# keyforge-data

Conventions for this repository. The vocabulary is in
[CONTEXT.md](CONTEXT.md). The decisions are in [docs/adr](docs/adr).

## Validation

- `mage ci:fix && mage ci:check` must print `ALL GREEN`.
- Tool configs are generated from `mklv.config.json`. Do not edit
  `.golangci.yaml`, `.gitignore`, or the other generated files by hand.
- Put scratch files in `./tmp`.

## Package layout

| Path                | Role                                                        |
| ------------------- | ----------------------------------------------------------- |
| `sets`              | The set table with the Master Vault expansion IDs           |
| `mv`                | The paced Master Vault client and the response parser       |
| `raw`               | The raw store: deck shards, linked objects, crawl manifests |
| `crawl`             | The crawl modes (feed scan, refresh) and the greedy cover   |
| `cmd/kfdata`        | The CLI that the `raw:*` mage targets run                   |
| `sources/mv`        | Committed crawl outputs, for example the cover deck lists   |
| `artists`           | The match of OCR readings to the known artists              |
| `tools/artistocr`   | The Swift OCR tool (macOS Vision) for the artist line       |
| `overlay`           | The format of the override files, and their lint            |
| `overrides`         | The override files: the overlay over the Master Vault data  |

## Rules

- **Keep raw data raw.** The crawler stores each deck and each linked object
  as Master Vault sent it. Do not decode and re-encode raw objects. Rules
  that interpret cards belong in distillation, not in the crawler.
- **Use one Master Vault client for a run.** The pacer learns the throttle
  limits, and a second client would start over and get throttled again.
- **A crawl continues.** Every crawl mode saves its state after each page or
  deck, and a run with the same crawl ID continues from that state. Keep it
  this way for a new mode.
- **The raw store is not in git.** It is in `./raw` and in
  `gs://mklv-keyforge-cards/raw/mv`. Use `mage raw:push` and `mage raw:pull`.
