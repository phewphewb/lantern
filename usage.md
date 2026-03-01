# Usage

## First-time setup

```bash
# 1. Scan the network and generate network.yaml
./router-configurator discover

# 2. Review and edit network.yaml if needed
#    Then validate it
./router-configurator validate --ping

# 3. Preview what setup will do (optional)
sudo ./router-configurator setup --dry-run

# 4. Run setup
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

Only `setup` requires root. All other commands run as the current user.

```
discover    no sudo
validate    no sudo
certs       no sudo
ls          no sudo
setup       sudo required
```
