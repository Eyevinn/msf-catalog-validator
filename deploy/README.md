# Deploying the web UI

The validator's web UI (`-serve`) is hosted at
<https://moqlivemock.demo.osaas.io/msf-catalog-validator/>, reverse-proxied by
the Caddy server that already fronts that VM.

It runs as a small systemd service bound to `127.0.0.1:8088`; Caddy strips the
`/msf-catalog-validator` prefix and proxies the rest to it. Because the page
uses relative URLs, no application-side base-path configuration is needed.

## Files

| File | Destination on the VM |
|------|-----------------------|
| `out/msf-catalog-validator-linux-amd64` | `/usr/local/bin/msf-catalog-validator` |
| `deploy/msf-catalog-validator.service`  | `/etc/systemd/system/msf-catalog-validator.service` |
| `Caddyfile` (workspace root)            | the VM's Caddy config (e.g. `/etc/caddy/Caddyfile`) |

## Build

```sh
make build-linux   # -> out/msf-catalog-validator-linux-amd64
```

## Install (run on the VM, as root)

The binary and unit file are uploaded to the home directory (e.g. via
`scp ... moq:`); move them into place and start the service:

```sh
install -m 0755 msf-catalog-validator /usr/local/bin/msf-catalog-validator
install -m 0644 msf-catalog-validator.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now msf-catalog-validator
systemctl status msf-catalog-validator --no-pager
```

## Caddy

Add the route (already present in the workspace `Caddyfile`):

```caddy
redir /msf-catalog-validator /msf-catalog-validator/
handle_path /msf-catalog-validator/* {
    reverse_proxy localhost:8088
}
```

Validate and reload (reload is atomic — a bad config is rejected without
dropping the running server):

```sh
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy   # or: caddy reload --config /etc/caddy/Caddyfile
```

## Upgrading

```sh
make build-linux
scp out/msf-catalog-validator-linux-amd64 moq:msf-catalog-validator
# on the VM:
install -m 0755 msf-catalog-validator /usr/local/bin/msf-catalog-validator
systemctl restart msf-catalog-validator
```
