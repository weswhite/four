# four

A terminal dashboard for tracking your portfolio allocations, dividends, and bucket strategy.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/weswhite/four/main/install.sh | sh
```

This downloads the latest release binary for your OS and architecture and installs it to `/usr/local/bin/four`.

## Usage

```
four [options] [portfolio.xlsx]
```

**Options:**
- `--source <path>` — Path to portfolio xlsx file
- `--set-source <path>` — Set default source and exit
- `--import <path>` — Import CSV/XLSX into bucket tracker
- `--legacy` — Force legacy single-portfolio view

On first run, the source file is saved to `~/.config/four/config.json` so you can just run `four` next time.
