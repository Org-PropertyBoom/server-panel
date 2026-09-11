> **Custody note.** Moved into this repo on 2026-09-10 from `design-templates`, which was only ever
> a temporary holder (the incident was written there because no better repo was open on the machine).
>
> Infra was nominally split to a CaddyDash hub on 2026-07-11, but **that repo is not on this machine
> and `caddydash.service` does not exist on the host** — this panel is what generates
> `/etc/caddy/caddy.json` (`services/caddy/`, see the 2026-09-07 entry in `.agents/works.md`), so the
> incident belongs with the code that owns the config. Related: `docs/tls-scaling.md`.

# Incident — shared Caddy down 5h45m, all tenants (2026-09-10)

**Host:** `ip-172-31-23-173` (AWS ap-southeast-1, public `52.76.123.15`)
**Component:** shared Caddy ingress, `caddy 2.6.2`, config `/etc/caddy/caddy.json`
**Down:** 2026-09-10 **00:06:14 UTC** → ~**05:50 UTC** (manual recovery; exact restore time not captured)
**Detected by:** a person opening a browser, 5½ hours in. **No alert fired.**
**Status:** resolved; outage class closed; three follow-ups open.

---

## Impact

**Every tenant on the shared ingress**, plus the control panel itself.

Confirmed down: `homeswithjo.com`, `launches.sg`, `rudyproperty.com`,
`singaporecondoreview.com` (Cloudflare **521**), **`cp.propertyweb.co`**.

The automation policy covering ~97 tenant domains and the one covering 10 infra hosts are both
served by this process, so the blast radius was the whole platform.

🔴 **`cp.propertyweb.co` is served *through* the Caddy it manages.** The control panel went down with
the data plane, so the repair surface was unavailable exactly when it was needed. Recovery was
SSH-only. That is a structural bootstrap problem, not bad luck.

## Root cause — five links, each independently fixable

1. **Let's Encrypt could not resolve DNS** for 8 tenant domains:
   `DNS problem: query timed out looking up A for launches.sg; no valid AAAA records found`
2. Both `http-01` and `tls-alpn-01` challenges failed → repeated **failed authorizations**
3. Five failures in an hour → **HTTP 429**, `too many failed authorizations (5) for "launches.sg"`
4. On that error path, **`acmez` in Caddy 2.6.2 type-asserts a nil authorization and panics** —
   `panic: interface conversion: interface {} is nil, not acme.Authorization`
5. The panic is unrecovered → **process dies** → **no `Restart=` policy** → stays dead until a human
   notices → **no alerting** → 5h45m

**The renewal ran during a TLS handshake** (`renewDynamicCertificate`), i.e. on-demand TLS, so a
single tenant's failing renewal executed inside the shared serving path.

### Panic (verbatim)

```
{"logger":"tls.renew","msg":"renewing certificate","identifier":"launches.sg","remaining":2021623}
panic: interface conversion: interface {} is nil, not acme.Authorization
  github.com/mholt/acmez.(*Client).ObtainCertificateUsingCSR   acmez/client.go:137
  certmagic.(*ACMEIssuer).doIssue                              acmeissuer.go:385
  certmagic.(*Config).renewCert.func2                          config.go:777
  certmagic.doWithRetry                                        async.go:104
  certmagic.(*Config).RenewCertAsync                           config.go:682
  certmagic.(*Config).renewDynamicCertificate.func3            handshake.go:628
systemd[1]: caddy.service: Main process exited, code=exited, status=2/INVALIDARGUMENT
```

Caddy had been up **17h 17m** before this. The config was and is valid — `caddy validate` would have
passed. This was a runtime crash, not a config fault.

## Why it lasted 5¾ hours

```
Restart=no          # systemd DEFAULT — never set by anyone
StartLimitBurst=5
```

The packaged unit at `/usr/lib/systemd/system/caddy.service` has **no `Restart=` line at all**
(verified: only `Type=notify`, `User=caddy`, `ExecStart=`). *(Corrected 2026-09-11: an earlier
draft said upstream's official unit ships `Restart=on-abnormal`. It does not: current
`caddyserver/dist` `init/caddy.service` has no `Restart=` either. The `restart.conf` drop-in below is
the only restart policy under either package.)*

**Nobody disabled restarts — Caddy never had a restart policy on this host.** The drop-in
`override.conf` only repoints `ExecStart` at `caddy.json`; it does not touch `Restart`.

## Fixed during the incident

**Restart policy** — `/etc/systemd/system/caddy.service.d/restart.conf`:

```ini
[Unit]
StartLimitIntervalSec=0

[Service]
Restart=always
RestartSec=10s
```

Verified: `Restart=always`, `RestartUSec=10s`, `StartLimitIntervalUSec=0`.
*(`StartLimitIntervalSec` is a `[Unit]` key — placing it under `[Service]` is silently ignored.)*

🔴 **`ExecReload` was aimed at the wrong config** — `/etc/systemd/system/caddy.service.d/reload.conf`:

```ini
[Service]
ExecReload=
ExecReload=/usr/bin/caddy reload --config /etc/caddy/caddy.json --force
```

The packaged unit had `ExecReload=/usr/bin/caddy reload --config /etc/caddy/Caddyfile --force`, while
`ExecStart` had been overridden to `caddy.json`. **Any `systemctl reload caddy` would have replaced
the running config for ~100 domains with `/etc/caddy/Caddyfile`, forcibly.** A second outage, one
reflexive command away. Now corrected; verified with `systemctl show caddy -p ExecReload`.

## Ruled out — do not re-chase these

| hypothesis | verdict | evidence |
|---|---|---|
| Bad vhost file / config error | ❌ | ran 17h before crashing; config valid |
| Someone deliberately disabled restarts | ❌ | no `Restart=` anywhere; systemd default |
| DNSSEC validation failure | ❌ | `dig @8.8.8.8` and `dig @8.8.8.8 +cd` both `NOERROR` |
| Shared broken nameserver | ❌ | 6 domains on **5 different** GoDaddy NS pairs, 2 on `webserver.sg` |
| Staging certs being served | ❌ | live cert is production LE `CN=YE2`, valid Jul 5 → Oct 3 2026 |
| Staging issuer misconfigured | ❌ | not in config; only issuer is `{"module":"acme","ca":null}` = LE **production** |

The `acme-staging-v02` orders in the log are **certmagic's built-in test-CA retry** after production
failures — deliberate, so retries burn staging's rate limits instead of production's. Diagnostic
noise; it never touched a served certificate.

The `no OCSP stapling ... no OCSP server specified` warnings are **benign** — Let's Encrypt stopped
including OCSP URLs in 2025.

## 🔴 OPEN — deadline 3 October 2026

**Eight domains are failing renewal.** All were issued together on 5 July and **all expire on 3
October, within four hours of each other** — one cohort, one cliff, eight sites breaking
simultaneously:

```
09:28  holland-linkresidences.com
09:39  kevinfeng.sg · launches.sg
13:23  faber-residence-condo.com · penrith-condo-sg.com · the-faberresidence.com
       the-sen-residence.com · the-zyon-grand.com
```

The cohort is also why the failures arrived as a burst — eight renewals entering the window at once,
all failing, exhausting the rate limit.

**The DNS failure is not reproducible from the host.** `dig +trace`, `dig @8.8.8.8` (with and without
`+cd`) all resolve correctly and fast. Let's Encrypt queries authoritative servers from multiple
continents and validates; the host queries a local caching resolver. Use **letsdebug.net** for an
outside-in read — it reproduces LE's checks. It may not be a fault you control.

**Update 2026-09-11: the DNS fault is in Let's Encrypt's multi-perspective validation, so it can't be
reproduced from here.** The host journal shows, for `launches.sg`:
`HTTP 403 urn:ietf:params:acme:error:caa - Error finalizing order :: rechecking caa: During secondary
validation: While processing CAA for launches.sg: DNS problem: networking error looking up CAA`.
"Secondary validation" is LE checking from several remote network perspectives. The primary
perspective succeeds; the remote ones can't reach the domain's nameservers (`ns1/ns2/ns3.webserver.sg`).
The host and 8.8.8.8 both resolve them, which is why every `dig` we ran came back clean. Scope is
**2 of the 8**: `launches.sg` and `kevinfeng.sg` are the two on `webserver.sg`. **The other 6 are on
GoDaddy nameservers (5 different pairs), and this does not explain them.** Their failure reason still
has to be read from the journal per domain. The fix for the two is at the DNS host (reachability of
`webserver.sg` from outside Singapore, or moving the domains' DNS), not on this server.

## Follow-on findings, 2026-09-11 (during the upgrade preparation)

**🔴 The Caddy admin API is open on every interface.** The host journal shows `"admin endpoint
started" "address":":2019" "enforce_origin":false` and `WARN "admin endpoint on open interface; host
checking disabled"`. Anyone who can reach port 2019 can `POST /load` with no authentication and replace
the config for ~100 domains. The AWS security group only covers the internet side. **`:2019` also
listens on the Docker bridge**, so any compromised tenant container can reach it no matter what the
security group says. **Fix: `admin localhost:2019`** in the hand-managed Caddyfile's global options,
with no `origins` and `enforce_origin` left off. This was checked against Caddy v2.6.2 source and the
panel's code before anyone edited anything:
- The panel's `caddy reload --config` takes the admin address from the file's `admin.listen` and sends
  `Host`/`Origin` for it.
- On a loopback listen, 2.6.2 turns host checking on and allows `localhost`, `127.0.0.1` and `[::1]` on
  port 2019.
- The panel's own `GET /config/` (default `CADDY_ADMIN_URL=http://localhost:2019`) sends no `Origin`
  header, so enabling `enforce_origin` would 403 it.

On 2.6.2 the change must go through a **cold restart**, never a panel reload (see the next item).

**A hot reload that removes the on-demand limiter fails on 2.6.2 and leaves disk and memory out of step.**
Journal: `http: panic serving 127.0.0.1:57036: invalid configuration: maxEvents = 0 and window != 0` via
`certmagic.(*RingBufferRateLimiter).SetMaxEvents` ← `caddytls.(*TLS).Provision tls.go:184` ←
`adminLoad.handleLoad`.
- The limiter is a package-level global that survives hot reloads. Go's `net/http` recovered the panic
  inside the `/load` handler, so the caller got `EOF` and the process (PID 479641, `NRestarts=0`) kept
  serving the **old** config.
- The panel had already written the **new** `caddy.json`, so disk and memory diverged. Every later hot
  reload fails the same way until a cold restart.
- Same class of bug as 2026-09-10, opposite outcome: yesterday's panic ran in a certmagic background
  goroutine where nothing recovers it, so it killed the process.
- The aborted provision leaked a certificate-maintenance goroutine on its own, empty, per-app cache. The
  running app's cache was untouched.

**The "Not reaching us" health probe is inverted and has no monitoring value.**
- It takes "this server" from the panel domain's DNS unless `CADDY_HEALTH_SERVER_IPS` is set.
  `cp.propertyweb.co` is behind Cloudflare (`104.21.69.82`, `172.67.206.132`).
- So **every Origin tenant fails "resolves to this server"** while serving normally. The only passes are
  Cloudflare-proxied tenants on the panel zone's IP pair, and their TLS completes at Cloudflare's edge,
  so they would stay green through an origin outage.
- On 2026-09-11: 96 hosts, 11 passing, 85 flagged, including all 31 Origin hosts.
- **Correction to the 2026-09-07 work log and to this doc's earlier reasoning:** "85 of 96 hosts don't
  resolve to this server" and "apss.com.sg doesn't resolve here" both came from this probe, so they
  were the bug, not DNS reality. `apss.com.sg` resolves to origin `52.76.123.15`.
- Host-side mitigation: pin `CADDY_HEALTH_SERVER_IPS=52.76.123.15,52.77.202.62` (this server's two Elastic IPs, ppt1 and ppt2). **The Edge column has the same blind spot:** `originIPs()` (`CUTOVER_ORIGIN_IPS`, default `52.76.29.0,52.76.123.15,3.1.252.222`) doesn't include ppt2 `52.77.202.62`, so ppt2 tenants such as `space-nova.com` and `spacenova.org` show as Elsewhere (both EIPs serve the same Caddy and the same certificate, confirmed from outside AWS) for the panel service. Code fix:
  one shared, correct definition of this server's IPs for both the probe and the Edge column, and alert only on hosts pointing here that fail TLS.

## Open follow-ups, in priority order

1. **Upgrade Caddy from 2.6.2** *(Nov 2022 — three years old)*. This is the highest-value fix: on a
   current Caddy the nil-authorization assertion is fixed, so a failed ACME path becomes a log line
   instead of a dead ingress. **It makes every other item on this list non-critical.**
2. **Resolve the 3 October renewal cohort** — see above.
3. **Alerting.** A port-443 check and a certificate-expiry check would each have caught this
   *independently* — the outage at 00:06, and the renewal failures three weeks before they bite.
   Every other item here is a fix for one fault; this is what catches the next unrelated one.

## Structural findings — bigger than this incident

**`caddydash.service` does not exist on this host.** `systemctl status` returns *"Unit
caddydash.service could not be found"* — yet the host runs a generated `/etc/caddy/caddy.json` and
serves a `/vhosts` + `/containers` control panel. **Something writes that config and it is not the
service the architecture says owns it.** Worth answering before the next vhost change goes through
whatever that something is.

**~97 tenant domains share a single automation policy with a single issuer.** There is no per-tenant
isolation in certificate issuance, which is precisely why one domain's ACME failure path became every
domain's failure path. A second policy covers 10 infra hosts with `"issuers": []` (Caddy defaults) —
so the two halves of the platform are provisioned differently.

**The control panel is served by the ingress it controls.** Restated here because it is the finding
most likely to matter during the *next* incident, whatever its cause.

## Verified in `server-panel` after the handover (2026-09-10)

Checks run against this repo to answer the two questions the write-up left open for it.

**The `ExecReload` trap was never armed from the panel.** Nothing in this repo calls
`systemctl reload caddy` — the only `systemctl` invocation in Go is a read-only
`systemctl is-active --quiet` (`services/apps.go:131`). Reloads shell the Caddy binary directly,
`caddy reload --config <path>` with `path` = the compiled `caddy.json`
(`services/caddy/caddyctl/compile.go`, `ReloadFromFile`), so they name the config explicitly and
never go through systemd's `ExecReload`. `services/caddy/caddyctl/adapt.go:10` records this as a
deliberate rule ("NEVER `systemctl reload`"). `public/install.sh:153` runs
`systemctl enable --now caddy`, which is a *start* (`ExecStart`, already overridden to `caddy.json`),
not a reload. **The fix to `reload.conf` was still correct and necessary** — it closed the trap for
anyone typing the command by hand, which is how it would have been sprung.

**Structural finding #1 confirmed: this panel generates `/etc/caddy/caddy.json`.** Since 2026-09-07
(`services/caddy/`, commit `9c5229a`) every apply adapts the vhost sources, validates the JSON,
writes it atomically to `CompiledConfigPath` (default `/etc/caddy/caddy.json`, env
`CADDY_COMPILED_CONFIG`) and then reloads *from that file*. The absent `caddydash.service` is not a
missing writer — there is no gap; the writer is this process.

**`docs/tls-scaling.md` had already named the mechanism** (written 2026-07-26, before this outage):

- Its rate-limit section lists *"5 failed validations per hostname per hour (**the dead-domain
  case**)"* as one of the three real scaling limits, and calls out that LE limits *"bite on **BURSTS**,
  not steady state"*. The 3 October cohort is exactly that shape — eight certs co-issued on 5 July
  renew as one burst. The doc's mitigation ("stagger") is aimed at *onboarding*; **nothing staggers a
  renewal cohort that a past bulk onboarding already created.** Staggering the re-issue of those eight
  is the durable fix, not just getting them renewed once.
- It also anticipated the bootstrap finding, in stronger terms than this write-up: *"Do not put the
  panel's own domain on on-demand — that's a circular dependency … keep `cp.propertyweb.co` a
  static/pinned block."* It records a prior event where a routine panel restart broke the `ask`
  endpoint and took `cp.propertyweb.co`, go3 and laravel3 to curl `000` **simultaneously**. So
  "the panel is served by the ingress it manages" is a **recurring, already-documented class** here,
  not a one-off observation from this incident.

## Answered 2026-09-11: renewal and the panic, on the upgrade target

Read from upstream source at the versions Caddy **v2.11.4** pins (certmagic **v0.25.3**, acmez
**v3.1.6**). This is **not** the 2.6.2 distro binary. That one is stripped, so its certmagic version
is unrecoverable, and a distro build may link a different certmagic than upstream 2.6.2 did.

- **Renewal re-consults the ask/permission endpoint.** `renewDynamicCertificate` calls
  `checkIfCertShouldBeObtained` before renewing, and on a deny removes the cert from the cache. So on
  the target, evicting a host from the ask allowlist (server-panel `2590ba9`) does stop renewal
  attempts; no separate purge of stored certs is needed.
- **The assertion that panicked is now checked.** The acmez v3.1.6 `ObtainCertificate` uses
  `authz, haveAuthz := problem.Resource.(acme.Authorization)`. That covers this one site only, not
  proof that no other panic path exists, so `restart.conf` stays.
- New exposure that comes with it: a **panel outage** now also fails renewal-window handshakes, not
  only uncached hosts. Details and the upgrade plan: `docs/caddy-upgrade-2.11.md`.

## Appendix — commands that produced this

```bash
systemctl status caddy caddydash --no-pager
journalctl -u caddy --since "2026-09-10 00:05" --until "2026-09-10 00:08" --no-pager | grep -m1 -B5 -A40 "panic:"
journalctl -u caddy --since "2026-09-09" --no-pager | grep "challenge failed" | grep -o '"identifier":"[^"]*"' | sort -u
systemctl show caddy -p Restart -p RestartUSec -p StartLimitIntervalUSec -p ExecReload
grep -nE "^(Restart|ExecStart|ExecReload|Type|User)" /usr/lib/systemd/system/caddy.service
sudo jq '.apps.tls.automation.policies[] | {subjects, issuers}' /etc/caddy/caddy.json
echo | openssl s_client -servername launches.sg -connect 127.0.0.1:443 2>/dev/null | openssl x509 -noout -issuer -dates
dig +trace launches.sg A ; dig @8.8.8.8 launches.sg A ; dig @8.8.8.8 +cd launches.sg A
```
