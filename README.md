# aliyunpan-cli

Agent-friendly Alibaba Cloud Drive CLI built on the Go `tickstep/aliyunpan-api` SDK.

This is an early MVP focused on OpenAPI-backed, scriptable operations. WebAPI-only
features such as share management, albums, and recycle-bin browsing are intentionally
reserved for a later phase.

## Install

```bash
go install github.com/harzva/aliyunpan-cli@latest
```

Or build locally:

```bash
go build ./...
```

## Auth

Import an existing `tickstep/aliyunpan` config:

```bash
aliyunpan-cli auth import --from ~/.config/aliyunpan/aliyunpan_config.json
```

Use your own Aliyun Drive OpenAPI OAuth application:

```bash
aliyunpan-cli auth login --client-id "$ALIYUNPAN_CLIENT_ID" --client-secret "$ALIYUNPAN_CLIENT_SECRET"
```

The login flow prints an authorization URL, then asks you to paste the returned code.
By default it uses `https://openapi.alipan.com/oauth/access_token` as the token
endpoint and `oob` as the redirect URI.

## Commands

```bash
aliyunpan-cli whoami
aliyunpan-cli drive list --format table
aliyunpan-cli drive use resource
aliyunpan-cli ls /
aliyunpan-cli stat /Documents/report.pdf
aliyunpan-cli mkdir /Backups
aliyunpan-cli upload ./report.pdf /Backups
aliyunpan-cli download /Backups/report.pdf --output ./report.pdf
aliyunpan-cli rm /Backups/report.pdf
```

Output defaults to JSON. Add `--format table` for human-readable output.
Upload and download progress is written to stderr by default; use `--json` or
`--no-progress` for clean non-progress command output.

## Config

Config is stored at:

```text
~/.config/aliyunpan-cli/config.json
```

Override the directory with:

```bash
ALIYUNPAN_CLI_CONFIG_DIR=/path/to/config aliyunpan-cli whoami
```
