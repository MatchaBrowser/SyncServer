# SyncServer
Sync server for Matcha browser

## Admin console

A small web console manages the invitations, shows the accounts, their devices and their last activity, and reads the audit journal of the security events (logins, password changes, revocations, with their IP address). It cannot read any synced data: everything is encrypted on the Macs.

It is disabled by default. To enable it, set `admin.listen` in `config.yaml`, on the loopback:

```yaml
admin:
  listen: 127.0.0.1:9090
```

Create an administrator: the password is drawn by the server and shown once.

```sh
./app admin user create alice -c config.yaml
```

Then reach the console through an SSH tunnel, or a VPN:

```sh
ssh -L 9090:127.0.0.1:9090 your-server
open http://127.0.0.1:9090
```

`admin user list`, `reset-password <name>` and `delete <name>` manage the administrators; `admin audit list` reads the journal from the command line. The pages, stylesheet and scripts (htmx) are embedded in the binary.
