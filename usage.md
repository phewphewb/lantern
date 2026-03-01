# Usage

## First-time setup

```bash
# 1. Generate a default network.yaml
./router-configurator init

# 2. Scan the network to fill in service IPs
./router-configurator discover

# 3. Validate the result
./router-configurator validate --ping

# 4. Preview what setup will do (optional)
sudo ./router-configurator setup --dry-run

# 5. Run setup
sudo ./router-configurator setup
```

After setup, point your router's DNS to this machine and install the CA
cert on each client device. Setup prints exact instructions at the end.

---

## Adding a new service

```bash
# Edit network.yaml manually, add the new service entry
# Then validate and rerun setup — it's idempotent
./router-configurator validate
sudo ./router-configurator setup
```

---

## Automatic IP monitoring (cron)

Services on DHCP can change IP. `sync` detects changes and reconfigures
automatically. Set `monitor.check_interval` in `network.yaml` and `setup`
installs the cron job for you:

```yaml
monitor:
  check_interval: 5m
  log_max_size: 10MB
```

Then rerun setup:

```bash
sudo ./router-configurator setup
# → Installing cron job... ✓  (*/5 * * * * ... sync --quiet)
```

`setup` is idempotent: rerunning it updates the interval if you change
`check_interval`, and removes the cron entry entirely if you delete the field.
Pass `--no-cron` to skip crontab management for a single run.

All write commands (including `sync`) log to `/var/log/router-configurator.log`
by default. The log file rotates automatically when it reaches `log_max_size`
(renamed to `.1`, new file started).

To test sync manually:

```bash
sudo ./router-configurator sync            # shows terminal output + writes log
sudo ./router-configurator sync --dry-run  # preview only, no changes
```

---

## Day-to-day commands

```bash
# See everything the tool manages and its current state
./router-configurator ls

# Check certificate expiry
./router-configurator certs

# Renew expiring certificates
./router-configurator certs renew

# Renew all certificates regardless of expiry
./router-configurator certs renew --all

# Check network.yaml is valid
./router-configurator validate

# Also ping each service to confirm reachability
./router-configurator validate --ping
```

---

## If something goes wrong

Setup automatically restores the previous config on failure. If that also
fails, it prints the backup path so you can restore manually:

```bash
# Backups are timestamped directories under:
ls /var/backups/router-configurator/

# To restore manually, copy files back:
sudo cp /var/backups/router-configurator/<timestamp>/frigate.home.conf \
        /etc/nginx/sites-enabled/
sudo systemctl restart nginx
```

---

## network.yaml reference

```yaml
version: 1
domain_suffix: home           # services become <name>.home
proxy_ip: 192.168.2.10        # IP of this machine
cert_warn_days: 30            # warn when certs expire within N days

monitor:
  check_interval: 5m          # setup installs/removes sync cron at this interval

services:
  - name: frigate
    ip: 192.168.2.10
    port: 5000
    websocket: true           # enable for camera streams

  - name: truenas
    ip: 192.168.2.20
    port: 80

  - name: mainsail
    ip: 192.168.2.30
    port: 80
    moonraker_port: 7125      # used by discover for fingerprinting
```

---

## Requires sudo

```
init        no sudo
discover    no sudo
validate    no sudo
certs       no sudo
ls          no sudo
setup       sudo required
sync        sudo required  (may trigger setup)
```
