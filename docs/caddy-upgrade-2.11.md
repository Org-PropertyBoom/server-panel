# Caddy 2.6.2 → 2.11.4: source migration runbook

**Owner decides timing and go/no-go.** Drafted 2026-09-11 in server-panel, from the design-templates
hub's host findings plus upstream source read at tag **`v2.11.4`** (published 2026-06-03). Related:
`docs/incident-2026-09-10-caddy-outage.md`, `docs/tls-scaling.md`.

> **Executed 2026-09-11, in one stopped window rather than in this document's order.** The Owner
> accepted up to 3 days of downtime, so the 2.6.2 intermediate cold restart (B5) was skipped. What ran:
> 1. While 2.6.2 still served: `interval`/`burst` removed; `admin localhost:2019` set; the Caddyfile
>    adapted with the downloaded 2.11.4 binary into a scratch file (native `permission` shape); `.admin`,
>    `on_demand` and a host count of 108 checked; the scratch file validated with that binary. Storage
>    checks (A4–A7) and backups (B1–B4) as written.
> 2. Live reconcile off, `systemctl stop caddy`, the scratch file copied over `/etc/caddy/caddy.json`,
>    D1–D5, then D6.
> 3. **D6 started Caddy by itself** (see the "When does the swap happen?" row), so the planned "validate
>    with the installed binary before the first start" couldn't happen. It was safe only because the
>    pre-install validate used a release binary whose module hash matched the `.deb`'s exactly
>    (`h1:XKxkMTgN…Wi0=`). After the install, a fresh `caddy adapt` diffed empty against `caddy.json`.
> 4. Verified: v2.11.4; admin on `127.0.0.1:2019`; the same certificate serial served (no re-issue);
>    four tenants return 200 from outside; host count 108; 127 certs on
>    disk (= baseline); 0 `obtaining certificate` in 15 min; live reconcile back ON and a panel Force
>    reload returned `reloaded: true`. After the swap the `/etc` drop-ins still applied:
>    `Restart=always`, and `ExecReload` → `caddy.json`.
>
> **For the next upgrade:** the pre-install validate **is** the gate. Validate with the exact binary from
> the `.deb` (`apt-get download caddy=<ver>`, then `dpkg-deb -x` it into a scratch directory), or with a
> release binary whose `caddy version` hash matches it. Expect Caddy to be running as soon as `apt-get
> install` returns. To validate the installed binary before it starts, the Debian way is a temporary
> `/usr/sbin/policy-rc.d` that exits 101 for the duration of the install, removed afterwards.
> `deb-systemd-invoke` consults it, so the postinst's `start` becomes a no-op. That hasn't been tested
> on this host.

## Why this is a migration, not an update

The host runs **Ubuntu's `universe` package `caddy 2.6.2-14`** (dpkg owns `/usr/bin/caddy`,
dynamically linked, stripped). `Installed == Candidate`, so `apt upgrade` will never move it. The fix
is to switch to Caddy's official apt repository. Same package name, same `/usr/bin/caddy` path, same
`caddy` user.

## What was verified before writing this (upstream source, v2.11.4)

| question | answer | source |
|---|---|---|
| Does the panel emit JSON that must match the new schema? | **No.** The panel renders **Caddyfile** snippets (`tls { on_demand; issuer acme }`) and shells a bare `caddy adapt`, so the JSON shape comes from **whichever binary is installed**. After the swap, the next reconcile emits 2.11-shape JSON on its own. | `services/caddy/render/render.go`, `services/caddy/caddyctl/adapt.go:50` |
| So what is actually at risk? | The **on-disk `caddy.json` produced by 2.6.2**, which 2.11.4 cold-starts from during `apt install`'s restart, *before* the panel has re-adapted anything. | — |
| Is a legacy JSON `ask` still honoured? | **Yes, it isn't silently dropped.** `Provision` converts `on_demand.ask` into `PermissionByHTTP{Endpoint: ask}`. It's marked *"Deprecated. WILL BE REMOVED SOON"*, and it's a hard error only if `ask` **and** `permission` are both set. | `modules/caddytls/tls.go`, `ondemand.go` |
| Does the ask request still fit `/internal/tls-ask`? | **Yes.** GET with query param `domain`, and any 2xx counts as allowed. The loopback guard still passes (Caddy calls `127.0.0.1`). | `ondemand.go` `PermissionByHTTP` |
| 🔴 Anything that would **reject** the old config? | **`on_demand.rate_limit` no longer exists**, and module config is decoded with `StrictUnmarshalJSON`, so an unknown field fails the load. It's present only if the Caddyfile had `interval`/`burst`, and **those are now hard adapt errors** (*"no longer supported, remove it from your config"*). Checked in A2/A3. | `context.go` `LoadModuleByID`, `httpcaddyfile/options.go` |
| Will certificates carry over? | Expected, **must be confirmed (A4–A7)**. Storage defaults to `file_system` at `$XDG_DATA_HOME/caddy`, else `$HOME/.local/share/caddy`. The official package creates user `caddy` with home `/var/lib/caddy`, and its unit sets no `Environment`, which gives **`/var/lib/caddy/.local/share/caddy`**. If the current install resolves elsewhere, the new one would re-issue ~100 certs at once. | caddyserver.com/docs/conventions, `dist/scripts/postinstall.sh` |
| Restart policy after the swap? | **The official unit has no `Restart=` either.** This corrects the incident doc: current upstream `dist/init/caddy.service` doesn't ship `on-abnormal`. `restart.conf` is the **only** restart policy under either package. It lives in `/etc` and survives. | `dist/init/caddy.service` |
| `ExecReload` after the swap? | The official baseline is `caddy reload --config /etc/caddy/Caddyfile --force`, **the same trap as before**. `reload.conf` must keep overriding it. Checked in E3. | `dist/init/caddy.service` |
| When does the swap happen? | **Inside `apt install`, and it starts Caddy even if it's stopped.** On every `configure` (install *and* upgrade) the postinst runs `if deb-systemd-helper --quiet was-enabled caddy.service; then … deb-systemd-invoke start caddy.service`, so an enabled unit gets started. The separate `try-restart` block only matters if it's already running. *(Corrected 2026-09-11: this row used to say only `try-restart`, from a summarised read of the script. The live upgrade showed Caddy starting during the install, and the verbatim script confirms why.)* That's why Phase C, **before** the install, is the gate. | `dist/scripts/postinstall.sh` |
| Hand-managed `/etc/caddy/Caddyfile`? | It's a dpkg **conffile**. Without `--force-confold` dpkg prompts, and taking the maintainer's version would **replace it with the default**. Always pass `--force-confold`. | dpkg |
| Is the 2.6.2 panic class gone? | The assertion that panicked (`acmez` `client.go:137` in the 2.6.2 build) is a **checked** assertion (`authz, haveAuthz := …`) in acmez **v3.1.6**, which 2.11.4 pins together with certmagic **v0.25.3**. That's this one site only, not proof that no other panic exists. `restart.conf` stays as the backstop. | `acmez@v3.1.6/client.go`, `caddy@v2.11.4/go.mod` |

## Rules for running this

- **Paste one command at a time.** Yesterday every paste landed twice. After each command, compare the
  output with **Expect**. If anything looks doubled or unfamiliar, **stop**.
- **⚠ WRITES** marks every command that changes the host. Everything else is read-only.
- Stay in the SSH session the whole time. The panel is served by this Caddy, so it drops out during
  the restart.
- Written for the root shell the host already uses (`root@…:/#`), so there's no `sudo`.

## Phase A: pre-flight (read-only)

Record the values marked **(record)**. Later phases compare against them.

**A1** Confirm the starting version.
```bash
caddy version
```
Expect `2.6.2` (possibly with a distro suffix).

**A2** Check the global on-demand block in the hand-managed Caddyfile.
```bash
grep -nE 'on_demand_tls|ask |interval|burst' /etc/caddy/Caddyfile
```
Expect an `on_demand_tls` line and an `ask http://127.0.0.1:…/internal/tls-ask` line. **If
`interval` or `burst` appear (they do on this host), don't edit yet.** They're removed in **B5**,
after the backups, as the first write of the window. B5 explains why the timing matters.

**A3** Check the compiled on-demand block.
```bash
jq '.apps.tls.automation.on_demand' /etc/caddy/caddy.json
```
Expect `{ "ask": "http://127.0.0.1:…/internal/tls-ask" }` **(record the endpoint)**. A `rate_limit`
key is expected if A2 showed `interval`/`burst`. It's confirmed on this host: on 2026-09-11, 2.11.4's
validate failed with `json: unknown field "rate_limit"`. **B5 removes it. Don't start Phase D until A3
shows no `rate_limit`.**

**A4** Check the service user's home.
```bash
getent passwd caddy
```
Expect a line ending `:/var/lib/caddy:/usr/sbin/nologin`. **STOP if the home is different.**

**A5** Check for environment overrides that move the data dir.
```bash
systemctl show caddy -p Environment
```
Expect `Environment=` (empty). **STOP if `XDG_DATA_HOME` or `HOME` is set.** Send the output over first.

**A6** Confirm where the certificates are.
```bash
ls /var/lib/caddy/.local/share/caddy/certificates/
```
Expect `acme-v02.api.letsencrypt.org-directory` (a staging directory may also appear; this host has none).
**STOP on `No such file or directory`.**

**A7** Count stored certificates.
```bash
find /var/lib/caddy/.local/share/caddy/certificates -name '*.crt' | wc -l
```
Expect about 100 or more **(record as N)**.

**A8** Record the effective unit.
```bash
systemctl cat caddy | grep -E '^# /|^(ExecStart|ExecReload|Restart|RestartSec)='
```
Expect the baseline `/usr/lib/systemd/system/caddy.service` plus `override.conf`, `restart.conf` and
`reload.conf`, with the drop-ins pointing at `caddy.json`.

**A9** Count the hosts in the live config.
```bash
jq '[.. | objects | .host? // empty | arrays | .[]] | unique | length' /etc/caddy/caddy.json
```
Expect about 108 **(record as H)**.

## Phase B: backups and rollback staging (writes only under /root)

**B1 ⚠ WRITES** Back up the config directory.
```bash
cp -a /etc/caddy /root/caddy-etc-pre-2.11
```
Expect no output. Check with `ls /root/caddy-etc-pre-2.11`, which should show `Caddyfile` and `caddy.json`.

**B2 ⚠ WRITES** Back up certificate storage. Nothing restores it automatically; this archive is insurance against a mass re-issue.
```bash
tar -C /var/lib/caddy -czf /root/caddy-data-pre-2.11.tgz .local/share/caddy
```
Expect no output.

**B3 ⚠ WRITES** Lock the archive down, since it holds private keys.
```bash
chmod 600 /root/caddy-data-pre-2.11.tgz
```
Check with `ls -lh /root/caddy-data-pre-2.11.tgz`: `-rw-------`, non-zero size.

**B4 ⚠ WRITES** Save the current package locally, so rollback doesn't depend on the Ubuntu archive.
```bash
cd /root && apt-get download caddy=2.6.2-14
```
Expect `Get:1 … caddy … 2.6.2-14`. Check with `ls /root/caddy_2.6.2-14_*.deb` **(record the filename)**.

**B5 ⚠ WRITES, by hand: remove `interval`/`burst`.** Skip this if A2 showed neither. It's the first
change to the live config, so do it only when you're ready to go straight through Phase C and D.

Why the timing matters (read from the Caddy 2.6.2 and certmagic v0.17.2 source). The limiter is **one
global bucket for every host**. It's checked **after** `ask` allows a host, and it's checked on
**handshake-time renewals** too. The 8 failing October domains are known hosts, so `ask` allows them,
and this limiter is the only Caddy-side throttle on starting their renewals. On 2.6.2, Let's Encrypt's
429 is the crash path, so don't leave 2.6.2 running without the limiter any longer than the window.
The limiter also cuts the other way: a limiter denial during renewal drops that cert from the cache,
so a burst can knock healthy hosts' certs out. Neither state is clean on 2.6.2. Spend as little time
as possible in either, and get onto 2.11.4, where the option doesn't exist and a 429 no longer panics.

> 🔴 **Never drop the limiter with a hot reload on 2.6.2. The load fails and leaves disk and memory
> out of step.** On 2026-09-11 an earlier version of this step used **Vhosts → Force reload** here. The
> reload failed with `EOF` on `/load`. **Caddy did not crash** (same PID, `NRestarts=0`) and kept
> serving the **old** config. Cause, from source (pending the journal's `panic serving … maxEvents`
> line): 2.6.2 keeps the limiter in a package-level global that survives hot reloads. Loading a config
> without `rate_limit` calls `SetMaxEvents(0)` while the old 2m window is still set, and certmagic
> v0.17.2 panics on that (`maxEvents = 0 and window != 0`). The panic happens inside the admin `/load`
> handler, and Go's HTTP server **recovers** it, logs it, and closes that one connection with no reply.
> That's the EOF. The process survives, but the panel has already written the new `caddy.json`, so
> **disk holds the new config while memory still runs the old one, and every later hot reload fails
> the same way.** A **cold** start is safe because a fresh process starts with window 0. So the
> transition from limiter to no limiter must be a `systemctl restart`. The panel's Force reload and
> live reconcile are both hot. Once the running Caddy has no limiter, hot reloads are safe again.

1. Open `/etc/caddy/Caddyfile` in an editor. Inside `on_demand_tls { … }`, delete only the
   `interval …` and `burst …` lines. Leave `ask` alone. **Don't use `sed` on this file.**
2. Re-run the A2 command. Expect the `ask` line and **no** `interval`/`burst`.
3. Adapt it with the **installed 2.6.2** binary into a scratch file (this doesn't touch the running Caddy):
   ```bash
   caddy adapt --config /etc/caddy/Caddyfile > /tmp/caddy-nolimit.json
   ```
   Expect no error.
4. Check the scratch file has no limiter:
   ```bash
   jq '.apps.tls.automation.on_demand' /tmp/caddy-nolimit.json
   ```
   Expect `ask` and **no `rate_limit`**.
5. Check it's valid on both versions, first the installed one, then the new one:
   ```bash
   caddy validate --config /tmp/caddy-nolimit.json
   ```
   ```bash
   /tmp/caddy validate --config /tmp/caddy-nolimit.json
   ```
   Expect `Valid configuration` from both. (This needs C1–C4 done first. Phase C's downloads are
   read-only for the service, so running them before B5 is fine.)
6. **⚠ WRITES.** Put it in place:
   ```bash
   cp /tmp/caddy-nolimit.json /etc/caddy/caddy.json
   ```
7. **⚠ WRITES 🔴 Cold restart.** This is the only safe way to drop the limiter on 2.6.2.
   ```bash
   systemctl restart caddy
   ```
8. Run `systemctl is-active caddy` (expect `active`) and the A9 host count (expect **H**).
9. **Don't use Force reload until step 8 passes.** After that it's safe, and a panel Force reload
   rewrites `caddy.json` the panel's own way. Then go straight into Phase C from C6.

B1 already holds the pre-edit `Caddyfile` and `caddy.json`, and both are valid on 2.6.2, so the
rollback steps don't change.

## Phase C: validate the new binary (no service change)

Nothing here touches the running Caddy. **If any check fails, stop.** See the GO / NO-GO note for the post-B5 state.

**C1 ⚠ WRITES /tmp** Download the official release.
```bash
cd /tmp && curl -fsSLO https://github.com/caddyserver/caddy/releases/download/v2.11.4/caddy_2.11.4_linux_amd64.tar.gz
```

**C2 ⚠ WRITES /tmp** Download the checksums.
```bash
curl -fsSLO https://github.com/caddyserver/caddy/releases/download/v2.11.4/caddy_2.11.4_checksums.txt
```

**C3** Verify the download.
```bash
sha512sum --ignore-missing -c caddy_2.11.4_checksums.txt
```
Expect `caddy_2.11.4_linux_amd64.tar.gz: OK`. **STOP on anything else.** If it says "no properly
formatted checksum lines", send the output over.

**C4 ⚠ WRITES /tmp** Extract only the binary.
```bash
tar -xzf caddy_2.11.4_linux_amd64.tar.gz caddy
```

**C5** Confirm the version.
```bash
/tmp/caddy version
```
Expect `v2.11.4 h1:…`.

**C6** 🔑 **Cold-start test:** the new binary against today's on-disk config.
```bash
/tmp/caddy validate --config /etc/caddy/caddy.json
```
Expect the last line `Valid configuration`. A warning about the deprecated `ask` is expected and
fine. **STOP on any error.** This is exactly what `apt install`'s restart would do.

**C7** 🔑 **Next-reconcile test:** what the panel will produce with the new binary.
```bash
/tmp/caddy adapt --config /etc/caddy/Caddyfile > /tmp/caddy-2.11.json
```
Expect no error (warnings are fine). **STOP on any error**, especially `interval`/`burst`.

**C8** Validate the result.
```bash
/tmp/caddy validate --config /tmp/caddy-2.11.json
```
Expect `Valid configuration`.

**C9** Count its hosts.
```bash
jq '[.. | objects | .host? // empty | arrays | .[]] | unique | length' /tmp/caddy-2.11.json
```
Expect **exactly H** (from A9).

**C10** Check its on-demand block.
```bash
jq '.apps.tls.automation.on_demand' /tmp/caddy-2.11.json
```
Expect a `permission` block with `"module": "http"` and the **same endpoint as A3**.

> **GO / NO-GO.** Go only if C3, C6, C7 and C8 pass, C9 equals H, and C10's endpoint equals A3's.
> Otherwise stop and send the output. The host is still on 2.6.2. If you did B5 and the install won't
> follow promptly, put the limiter back: copy `/root/caddy-etc-pre-2.11/Caddyfile` over
> `/etc/caddy/Caddyfile`, then **Vhosts → Force reload**.

## Phase D: the swap (Caddy restarts once, for a few seconds)

**D1 ⚠ WRITES** Install the repository signing key.
```bash
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
```
Expect no output. If gpg asks `Overwrite? (y/N)`, the paste doubled: answer `N` and run it once more.

**D2 ⚠ WRITES** Add the repository. This uses `-o`, not `tee`, so a doubled paste overwrites instead of appending.
```bash
curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt -o /etc/apt/sources.list.d/caddy-stable.list
```
Check with `cat /etc/apt/sources.list.d/caddy-stable.list`: one `deb` and one `deb-src` line, each with
`signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg`, **not repeated**.

**D3 ⚠ WRITES** Make both files world-readable for apt.
```bash
chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg /etc/apt/sources.list.d/caddy-stable.list
```

**D4 ⚠ WRITES** Refresh package lists.
```bash
apt update
```
Expect a line for `dl.cloudsmith.io/public/caddy/stable` and **no GPG error**.

**D5** Confirm the candidate.
```bash
apt-cache policy caddy
```
Expect `Installed: 2.6.2-14` and **`Candidate: 2.11.4`** from `dl.cloudsmith.io`. **STOP if the
candidate is any other version.** Redo Phase C for that version first, because a validated binary
must be the installed binary.

**D6 ⚠ WRITES 🔴 THE SWAP.** Installs 2.11.4 and starts or restarts Caddy. The postinst starts an enabled unit even if it was stopped. Use the exact version string D5 printed.
```bash
apt-get install -y -o Dpkg::Options::=--force-confold caddy=2.11.4
```
Expect unpack/setup lines and no conffile question. `--force-confold` keeps the hand-managed Caddyfile.

## Phase E: post-checks (read-only unless marked)

**E1** Confirm the new version.
```bash
caddy version
```
Expect `v2.11.4 h1:…`.

**E2** Confirm the service is up.
```bash
systemctl is-active caddy
```
Expect `active`. **If it's `activating` or `failed`, go to Rollback now.** `Restart=always` will retry
every 10 seconds, so this isn't a dead end, but don't wait on it.

**E3** Confirm the drop-ins still win over the new baseline.
```bash
systemctl show caddy -p ExecStart -p ExecReload -p Restart -p RestartUSec
```
Expect both `ExecStart` and `ExecReload` to contain `caddy.json`, plus `Restart=always` and
`RestartUSec=10s`. **If `ExecReload` shows `Caddyfile`, don't run `systemctl reload caddy`.** Send the
output over.

**E4** Confirm every host loaded.
```bash
curl -s localhost:2019/config/ | jq '[.. | objects | .host? // empty | arrays | .[]] | unique | length'
```
Expect **exactly H**.

**E5** Confirm the panel answers over TLS.
```bash
curl -sI https://cp.propertyweb.co | head -1
```
Expect `HTTP/2 200` or a redirect (3xx). Not a 5xx or a connection error.

**E6** Confirm certificates were reused, not re-issued.
```bash
find /var/lib/caddy/.local/share/caddy/certificates -name '*.crt' | wc -l
```
Expect **exactly N**.

**E7** Watch for a mass issuance.
```bash
journalctl -u caddy --since "15 min ago" | grep -c "obtaining certificate"
```
Expect 0 or low single digits (only hosts genuinely due). **If it reads in the tens and climbs on
re-run, storage wasn't picked up. Roll back before the rate limiter does it for you.**

**E8** Panel round-trip, in the panel: **Vhosts → Force reload.** Expect `reloaded: true` and
`compiled_path: /etc/caddy/caddy.json`. This is the first adapt by the new binary.

**E9** Confirm the config changed shape.
```bash
jq '.apps.tls.automation.on_demand' /etc/caddy/caddy.json
```
Expect the `permission` / `"module": "http"` shape (same as C10). **From here on the on-disk config is
2.11-only.** See the rollback note.

**E10** Review recent errors.
```bash
journalctl -u caddy --since "15 min ago" | grep -iE "panic|error" | tail -20
```
Expect no `panic`. ACME errors for the 3 October cohort are expected. On 2.11.4 they should be log lines, not a crash.

## Rollback

> **Order matters.** After **E8**, `caddy.json` uses the `permission` field, which 2.6.2 doesn't
> know. Its strict decoder would refuse the file and 2.6.2 would crash-loop on it. **Restore the
> pre-migration JSON before downgrading.**

**R1 ⚠ WRITES** Restore the pre-migration compiled config.
```bash
cp /root/caddy-etc-pre-2.11/caddy.json /etc/caddy/caddy.json
```

**R2 ⚠ WRITES 🔴** Reinstall 2.6.2 from the saved package. Use the filename recorded in B4.
```bash
apt-get install -y --allow-downgrades -o Dpkg::Options::=--force-confold /root/caddy_2.6.2-14_amd64.deb
```

**R3 ⚠ WRITES** Restart explicitly. Don't rely on the downgrade's maintainer scripts to do it.
```bash
systemctl restart caddy
```

**R4** Verify: `caddy version` should show `2.6.2`, `systemctl is-active caddy` should show `active`,
and the E4 command should give **H**.

**R5 ⚠ WRITES** Stop the next `apt upgrade` from reinstalling 2.11.4.
```bash
apt-mark hold caddy
```
Expect `caddy set on hold.`

**R6** In the panel: **Vhosts → Force reload.** This re-adapts with 2.6.2 and replaces the restored
backup with current vhosts. The backup predates the window.

Only if certificate storage itself was damaged: stop Caddy, restore `/root/caddy-data-pre-2.11.tgz`
into `/var/lib/caddy`, `chown -R caddy:caddy /var/lib/caddy/.local`, then start. Not a default step.

## After a successful migration

- **Recommended: `apt-mark hold caddy`.** Otherwise every `apt upgrade` moves Caddy on its own, and
  every version change needs Phase C again: the panel compiles against whichever binary is installed,
  and module config is decoded strictly. The Owner decides.
- Delete `/tmp/caddy*`. Keep the `/root` backups for about a week, then delete the `.tgz` (private keys).
- **Renewal behaviour changes, and that's mostly good.** In certmagic v0.25.3, `renewDynamicCertificate`
  calls `checkIfCertShouldBeObtained` before renewing. So **the permission endpoint is consulted on
  renewal**, and evicting a host from the ask allowlist (commit `2590ba9`) now does stop renewal
  attempts for it. On a deny, certmagic removes the certificate from its cache ("it will be deleted
  from storage later").
- ⚠ **New exposure from the same change** (read from source, not tested): if the **panel is down**
  when an on-demand cert is due and gets a handshake, the permission check fails. That cert is dropped
  from the cache, and the host fails TLS until the panel is back. `docs/tls-scaling.md` already lists
  "panel down" as a residual risk; on 2.11.4 it also reaches hosts **in their renewal window**, not
  only uncached ones. Keep panel restarts short, and treat panel-down alerting as TLS alerting.
- The 3 October cohort becomes non-critical (a failed renewal is a log line), but those certs still
  expire. Fix or remove those domains separately.
