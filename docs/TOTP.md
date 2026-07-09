# Two-factor authentication (TOTP)

Restreamer and datarhei Core support optional TOTP (time-based one-time password) as a second factor at login. Enrollment and management are available in the Restreamer UI under **Settings → Authentication**, or via the `/api/v3/auth/totp` API endpoints.

TOTP state is stored on disk in the **database directory** (`db.dir`, environment variable `CORE_DB_DIR`). In a typical Restreamer Docker setup this is the same volume as the config directory (`/core/config`, often bind-mounted to `/opt/restreamer/config` on the host).

| File | Purpose |
|------|---------|
| `totp.json` | TOTP enrollment (secret and status) |
| `totp_trust.json` | Remembered devices that may skip TOTP until expiry |

There is no in-app “forgot TOTP” flow and no backup codes. If you lose access to your authenticator app, you must reset TOTP on the server (see below).

## Disable TOTP (normal)

If you still have your authenticator app:

1. Log in with username, password, and TOTP code.
2. Open **Settings → Authentication**.
3. Click **Disable TOTP** and confirm with a code from the app.

Alternatively, send `DELETE /api/v3/auth/totp` with a valid JWT and the current TOTP code in the request body.

## Reset TOTP when the code is lost

You need **host or container filesystem access** to the database directory. You still need the Restreamer **password** unless you reset that separately (see [Password also lost](#password-also-lost)).

TOTP data is read when Core starts. After deleting the files, **restart Core** (or the Restreamer container) for the change to take effect.

### Restreamer Docker (typical bind mount)

Official install examples mount config like this:

```sh
-v /opt/restreamer/config:/core/config
```

On the host:

```sh
sudo rm /opt/restreamer/config/totp.json
sudo rm /opt/restreamer/config/totp_trust.json   # optional; clears remembered devices
docker restart restreamer
```

Adjust the host path if your mount differs. `totp_trust.json` is optional to remove; deleting it only invalidates “remember this device” tokens.

### datarhei Core Docker

If you run Core directly (see [README](../README.md)):

```sh
rm "${HOME}/core/config/totp.json"
rm "${HOME}/core/config/totp_trust.json"   # optional
docker restart core
```

### Local development

When running Core with `make run` from this repository, the default database directory is `./config`:

```sh
rm config/totp.json
rm config/totp_trust.json   # optional
# restart the Core process
```

### Docker without a bind mount

If the container was started without `-v`, config lives in a Docker volume. Find the mount point:

```sh
docker inspect restreamer --format '{{range .Mounts}}{{if eq .Destination "/core/config"}}{{.Source}}{{end}}{{end}}'
```

Then remove the files under that path on the host and restart the container. Or exec into the container:

```sh
docker exec restreamer rm /core/config/totp.json
docker exec restreamer rm /core/config/totp_trust.json   # optional
docker restart restreamer
```

### After reset

1. Log in with **username and password only** (TOTP is no longer required).
2. Re-enable TOTP in **Settings → Authentication** if you want two-factor protection again.

## Trusted device without the authenticator

If you previously checked “remember this device” on a browser and the trust period has not expired, you may be able to log in from that browser with **password only**. Trust is stored in `totp_trust.json` on the server and in the browser’s local storage. This does not help on a new device or after trust expires.

## Password also lost

TOTP reset does not change the login password. To reset the password:

- Edit `config.json` in the config/database directory (`api.auth.password`), or
- Set environment variable `CORE_API_AUTH_PASSWORD` or `RS_PASSWORD` and restart Core.

The Restreamer UI password wizard only runs on first setup when authentication has never been enabled; it is not a forgot-password flow.

## Security note

Removing `totp.json` is a break-glass recovery for administrators with filesystem access. Anyone who can modify files in `db.dir` can disable TOTP. Protect that directory and container/host access accordingly.
