# Installing a server-panel build onto production

## Rule: announce before install (INSTALL-ANNOUNCE-1)

Owner ruling, 2026-09-11.

**Any session that installs a server-panel build onto prod announces it to the other active
sessions FIRST, naming the build number, and only then installs.**

Why: every install restarts the panel process. Sessions are held in memory, so **every operator is
logged out** and anything they were doing in the panel is cut off. On 2026-09-11 two sessions
installed builds onto prod minutes apart (`20260911034423`, then `20260911035628`), and neither
warned the other.

### What counts as an install

- Panel header → **Check Update → Update and restart** (`POST /post/update`).
- Calling `POST /post/update` directly, from a script or a browser console.
- Re-running `install.sh` on the host, including `--reinstall`.
- Swapping the binary by hand and restarting `ppt-server-panel@root`.

Pushing to `main` is **not** an install. Neither is CI publishing to `server-panel-dist`: that only
makes a build *available*. Nothing changes on the server until one of the actions above runs.

### How to announce

1. Get the build number you are about to install: `GET /post/update` → `remoteVersion`
   (e.g. `20260911035628`).
2. List the active sessions (`list_sessions`). Message (`send_message`) each running session that
   works on ppt-platform. At minimum, message the hub (Server Architect / The Hub - Architect).
3. The message says: the build number, the commits it carries, and that installing will restart
   the panel and log everyone out.
4. Then install. If a session objects, stop and resolve it first.
5. After the restart, confirm `GET /post/update` → `localVersion` equals the build you announced.

Sessions are logged out by the restart, so the operator has to log in again afterwards. Say so in
the announcement.
