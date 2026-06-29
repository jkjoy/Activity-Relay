# Activity Relay Server

## Yet another powerful customizable ActivityPub relay server written in Go.

[![GitHub Actions](https://github.com/yukimochi/activity-relay/workflows/Test/badge.svg)](https://github.com/yukimochi/Activity-Relay)
[![codecov](https://codecov.io/gh/yukimochi/Activity-Relay/branch/master/graph/badge.svg)](https://codecov.io/gh/yukimochi/Activity-Relay)

![Powered by Ayame](docs/ayame.png)

## Packages

 - `github.com/yukimochi/Activity-Relay`
 - `github.com/yukimochi/Activity-Relay/api`
 - `github.com/yukimochi/Activity-Relay/deliver`
 - `github.com/yukimochi/Activity-Relay/control`
 - `github.com/yukimochi/Activity-Relay/models`

## Requirements

 - [Redis](https://github.com/redis/redis)

## Run

### API Server

```bash
relay --config /path/to/config.yml server
```

### Job Worker

```bash
relay --config /path/to/config.yml worker
```

### CLI Management Utility

```bash
relay --config /path/to/config.yml control
```

## Config

### YAML Format

```yaml config.yml
ACTOR_PEM: /var/lib/relay/actor.pem
REDIS_URL: redis://redis:6379

RELAY_BIND: 0.0.0.0:8080
RELAY_DOMAIN: relay.toot.yukimochi.jp
RELAY_SERVICENAME: YUKIMOCHI Toot Relay Service
JOB_CONCURRENCY: 50
# RELAY_SUMMARY: |

# RELAY_ICON: https://
# RELAY_IMAGE: https://
```

### Environment Variable

 **Optional** : When config file not exist, use environment variables.

 - ACTOR_PEM
 - REDIS_URL
 - RELAY_BIND
 - RELAY_DOMAIN
 - RELAY_SERVICENAME
 - JOB_CONCURRENCY
 - RELAY_SUMMARY
 - RELAY_ICON
 - RELAY_IMAGE
 - ADMIN_ENABLED
 - ADMIN_USERNAME
 - ADMIN_PASSWORD

## Built-in Web Admin and Public List

Activity-Relay includes a small built-in web UI in the API server:

- `/` public relay landing page with inbox/actor usage instructions.
- `/servers` public relay member list.
- `/api/servers` public JSON member list.
- `/admin` and `/admin/servers` visual admin panel.

Admin mutations use HTTP Basic authentication when `ADMIN_ENABLED` is true:

```yaml
ADMIN_ENABLED: true
ADMIN_USERNAME: admin
ADMIN_PASSWORD: change-me
```

The admin panel can delete active memberships, mark domains as limited, block domains, and unblock/unlimit existing domain rules. Enable the admin panel only behind HTTPS and use a strong password.

## How to Use Relay (for Relay Customers)

### Mastodon, Misskey and their forks

Subscribe this inbox `https://<your-relay-server-address>/inbox`

### Pleroma and their forks

Follow this actor `https://<your-relay-server-address>/actor`

## [Document](https://github.com/yukimochi/Activity-Relay/wiki)

See [GitHub wiki](https://github.com/yukimochi/Activity-Relay/wiki) to build / install / control relay.

## License

GNU AFFERO GENERAL PUBLIC LICENSE

## Project Sponsors

Thank you for your support!

### Monthly Donation

- [Patreon](https://www.patreon.com/yukimochi)
- [pixivFANBOX](https://yukimochi.fanbox.cc)
- [fantia](https://fantia.jp/fanclubs/11264)

**[My Donor List](https://relay.toot.yukimochi.jp#patreon-list)**

### Open Source Support Program

- [JetBrains for Open Source](https://jb.gg/OpenSourceSupport)
