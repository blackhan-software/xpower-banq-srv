# XPower Banq Server

Production server infrastructure for the XPower Banq DeFi protocol on Avalanche blockchain.

## Overview

This repository contains a complete production-ready server deployment for XPower Banq operations, featuring:

- **REST API Service** - High-performance Go API for utilization rates and price quotes
- **Blockchain Indexing** - Automated event indexing and TWAP calculations
- **Mining Service** - Containerized XPower cryptocurrency miner
- **Production Infrastructure** - Systemd services, Nginx configuration, monitoring

### Architecture

```
┌─────────────────────────────────────────┐
│   Nginx Reverse Proxy                   │
│   - Rate limiting (100/s)               │
│   - Gzip compression                    │
│   - SSL/TLS termination                 │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Docker: banq-api (Port 8001)          │
│   - Go + Chi router                     │
│   - Read-only SQLite access             │
│   - CORS middleware                     │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   SQLite Databases (/srv/db)            │
│   - ri_*.db (Interest Rates Indexed)    │
│   - rt_*.db (Exchange Rates Tracked)    │
└─────────────────────────────────────────┘
                  ↑
┌─────────────────────────────────────────┐
│   Systemd Services (Scheduled)          │
│   - banq CLI (Deno)                     │
│   - Blockchain indexing                 │
│   - TWAP calculation                    │
│   - Database ingestion                  │
└─────────────────────────────────────────┘
                  ↑
┌─────────────────────────────────────────┐
│   Avalanche Blockchain                  │
│   - Smart contracts (v10a)              │
│   - Event logs                          │
└─────────────────────────────────────────┘
```

## Repository Structure

The repository follows the Unix Filesystem Hierarchy Standard (FHS):

```
banq-srv/
├── docker/              # Docker containerization
│   ├── xpowerbanq/      # XPower Banq API service
│   │   └── banq-api/    # Go-based REST API
│   │       ├── source/  # Go source code
│   │       └── Docker.md
│   └── xpowermine/      # XPower mining service
│       └── miner/       # Deno-based miner
│           └── Docker.md
├── etc/                 # Configuration files
│   ├── banq/            # Application configs
│   │   ├── banq.env.mainnet
│   │   └── banq.env.testnet
│   ├── nginx/           # Nginx reverse proxy
│   │   └── sites-available/default
│   ├── profile.d/       # Shell environment
│   │   └── banq.sh
│   └── systemd/         # Service definitions
│       └── system/      # Service/timer files
├── usr/                 # User programs
│   └── local/bin/       # Executables and scripts
│       ├── banq-ri.sh
│       ├── banq-riw.sh
│       ├── banq-riw2db.sh
│       ├── banq-rt.sh
│       ├── banq-rtw.sh
│       └── banq-rtw2db.sh
└── var/                 # Variable data
    └── lib/banq/        # Database storage
```

## Components

### 1. XPower Banq API Service

**Location:** `docker/xpowerbanq/banq-api/`

A minimal, secure Go-based REST API providing read-only access to XPower Banq utilization rates and price quotes.

**Key Features:**
- Minimal memory footprint (~5-10MB at idle)
- Chi router with radix tree optimization
- Connection pooling for high throughput
- Hardcoded SQL queries (SQL injection prevention)
- Read-only database access
- Non-root Docker container (UID 1001)
- Comprehensive test coverage (52.2%)

**Documentation:**
- [API Docker.md](docker/xpowerbanq/banq-api/Docker.md) - Complete guide (deployment, development, testing)

**Technology Stack:**
- Go 1.21
- Chi v5 HTTP router
- SQLite3 with connection pooling
- Docker containerization

**API Endpoints:**
- `GET /health` - Health check
- `GET /{dbName}/daily_average` - Daily utilization rates
- `GET /{dbName}/daily_ohlc` - Daily OHLC price quotes

### 2. Blockchain Indexing Services

**Location:** `etc/systemd/system/`

Systemd-managed services for indexing blockchain events and calculating Time-Weighted Average Prices (TWAP).

**Service Categories:**
- **Template Services** - Execute as `banq:banq` user (non-root)
- **Wrapper Services** - Execute as root to orchestrate template instances
- **API Service** - Execute as root (manages Docker container running as `banq:banq`)

**Service Types:**

**Rate Index Services (RI):**
- `banq-ri@.service` - Template service for indexing blockchain events (oneshot)
  - Instance format: `banq-ri@TOKEN:MODE:POOL.service` (e.g., `banq-ri@APOW:supply:P000.service`)
  - Parameter parsing: TOKEN:MODE:POOL separated by `_` or `:`
- `banq-riw@.service` - Watch mode template (long-running, 86400-block scanning)
  - Pipes output to `/usr/local/bin/banq-riw2db.sh` for database ingestion
  - Database: `/var/lib/banq/ri-TOKEN:MODE:POOL.db`
  - 28 instances total (4 pools × 7 tokens × 2 modes: supply/borrow)
- Wrapper services (`banq-ri.service`, `banq-riw.service`) - Orchestrate all template instances
- Tracks supply/borrow rates across pools (P000-P006)
- Tokens: APOW, XPOW, AVAX, USDC, USDT

**Rate Tracker Services (RT):**
- `banq-rt@.service` - Template service for calculating Time-Weighted Average Prices (oneshot)
  - Instance format: `banq-rt@TOK0_TOK1:ORACLE.service` (e.g., `banq-rt@XPOW_APOW:T000.service`)
  - Parameter parsing: TOK0_TOK1:ORACLE separated by `_` or `:`
- `banq-rtw@.service` - Watch mode template (long-running, 86400-block scanning)
  - Pipes output to `/usr/local/bin/banq-rtw2db.sh` for database ingestion
  - Database: `/var/lib/banq/rt-TOK0_TOK1:ORACLE.db`
  - 14 instances total (7 token pairs × 2 directions)
- Wrapper services (`banq-rt.service`, `banq-rtw.service`) - Orchestrate all template instances
- Tracks token pair prices across oracles (T000-T006)
- Pairs: XPOW/APOW, AVAX/APOW, USDC/APOW, USDT/APOW

**Common Characteristics:**
- Template services load environment from `/etc/banq/banq.env.mainnet`
- 2-second startup delay prevents nonce conflicts when multiple instances start in parallel
- Batch processing services scan blockchain over last 86400 blocks
- Auto-restart on failure with 10-second delay
- Resource limits: 100% CPU quota, 500MB memory maximum

**Technology Stack:**
- Deno-compiled `banq` CLI binary
- Bash orchestration scripts
- SQLite databases (WAL mode)
- Systemd timers for scheduling

**Scheduling (Systemd Timers):**
- `banq-ri.timer` - Twice daily at 06:30, 18:30
- `banq-riw.timer` - Twice daily at 06:45, 18:45
- `banq-rt.timer` - Hourly at the top of each hour
- `banq-rtw.timer` - Twice daily at 06:15, 18:15
- All timers include 0-60 second randomized delay
- Persistent timers catch up on missed runs

### 3. XPower Mining Service

**Location:** `docker/xpowermine/miner/`

Containerized Deno-based cryptocurrency mining service for XPower tokens.

**Key Features:**
- Multi-worker mining (configurable)
- Automatic/custom gas parameter configuration
- Avalanche blockchain integration
- Cron-scheduled operations via supercronic

**Documentation:**
- [Miner Docker.md](docker/xpowermine/miner/Docker.md) - Docker setup and configuration

**Technology Stack:**
- Deno runtime
- Alpine Linux base image
- Supercronic scheduler

**Configuration:**
- Provider: Avalanche RPC endpoint
- Contract: v10a (configurable)
- Mining workers: 7 (default)
- Mint level: 8 (proof-of-work difficulty)

**WARNING:** Mining can seriously damage your device due to intensive CPU usage.

## Dependencies

### System Requirements

**For API Service:**
- Docker
- 1 CPU core
- 500MB RAM

**For Systemd Services:**
- Linux with systemd
- Dedicated `banq:banq` system user/group (UID/GID 1001 for Docker compatibility)
- `/usr/local/bin/banq` - Banq CLI binary (Deno-compiled)
- `/usr/local/bin/banq-ri.sh` - Reindex wrapper script
- `/usr/local/bin/banq-riw.sh` - Reindex watch wrapper script
- `/usr/local/bin/banq-riw2db.sh` - Reindex to database script
- `/usr/local/bin/banq-rt.sh` - TWAP wrapper script
- `/usr/local/bin/banq-rtw.sh` - TWAP watch wrapper script
- `/usr/local/bin/banq-rtw2db.sh` - TWAP to database script
- `/etc/banq/banq.env.mainnet` - Environment configuration file (readable by banq user)
- `/var/lib/banq/` - State directory for database files (writable by banq user)

**For Miner Service:**
- Docker
- Multi-core CPU (7+ cores recommended)

**For Production:**
- Nginx (reverse proxy, rate limiting, SSL)

## Quick Start

### Prerequisites

- Docker (for API and miner services)
- Systemd (for indexing services)
- Nginx (recommended for production)
- Deno-compiled `banq` CLI binary

### 1. API Service

```bash
# Build the Docker image
./docker/xpowerbanq/banq-api/build.sh

# Run with resource limits
docker run -d --name banq-api \
  --cpus=1.0 --memory=500m --memory-swap=500m \
  -v /var/lib/banq:/var/lib/banq:rw \
  -v /srv/db:/srv/db:ro \
  -p 8001:8001 \
  xpowerbanq/banq-api

# Test the API
curl http://localhost:8001/health
```

See [docker/xpowerbanq/banq-api/Docker.md](docker/xpowerbanq/banq-api/Docker.md) for detailed deployment instructions.

### 2. Systemd Services (Step-by-Step)

#### Step 1: Create System User

```bash
# Create banq user and group (UID/GID 1001 for Docker compatibility)
sudo groupadd -g 1001 banq
sudo useradd -u 1001 -g banq -s /bin/false -d /var/lib/banq -M banq
```

**Note:** The `-M` flag prevents home directory creation. UID/GID 1001 matches the user inside the Docker container.

#### Step 2: Install Banq CLI and Scripts

```bash
# Install banq CLI binary (adjust source path as needed)
sudo cp /tmp/banq-mainnet.x86_64-linux.run /usr/local/bin/banq

# Install wrapper scripts
sudo cp usr/local/bin/banq-*.sh /usr/local/bin/
```

#### Step 3: Set Script Permissions

```bash
# Executables (r/x by all, r/w only by root)
sudo chown root:root /usr/local/bin/banq /usr/local/bin/banq-*.sh
sudo chmod 755 /usr/local/bin/banq /usr/local/bin/banq-*.sh
```

#### Step 4: Create Required Directories

```bash
# Configuration directory
sudo mkdir -p /etc/banq

# State directory for database files
sudo mkdir -p /var/lib/banq

# Database symlink directory (for API service)
sudo mkdir -p /srv/db
```

#### Step 5: Configure File Permissions

```bash
# Configuration directory (r/o by all, r/w only by root)
sudo chown root:root /etc/banq
sudo chmod 755 /etc/banq

# Environment configuration (r/o by banq user)
sudo chown banq:banq /etc/banq/banq.env.mainnet
sudo chmod 640 /etc/banq/banq.env.mainnet

# State directory (r/w by banq user, r/o by all)
sudo chown banq:banq /var/lib/banq
sudo chmod 755 /var/lib/banq

# Database files (r/w by banq, r/o by all for API access)
sudo chown banq:banq /var/lib/banq/*.db* 2>/dev/null || true
sudo chmod 644 /var/lib/banq/*.db* 2>/dev/null || true

# Database symlink directory (r/o by all, r/w only by root)
sudo chown root:root /srv/db
sudo chmod 755 /srv/db

# Optional: Install shell profile
sudo cp etc/profile.d/banq.sh /etc/profile.d/
```

#### Step 6: Install Systemd Service Files

```bash
# Copy service and timer files
sudo cp etc/systemd/system/*.service /etc/systemd/system/
sudo cp etc/systemd/system/*.timer /etc/systemd/system/

# Reload systemd to recognize new services
sudo systemctl daemon-reload
```

#### Step 7: Enable and Start Services

```bash
# Start the API service
sudo systemctl enable --now banq-api.service

# Enable timers for scheduled execution
sudo systemctl enable --now banq-ri.timer
sudo systemctl enable --now banq-riw.timer
sudo systemctl enable --now banq-rt.timer
sudo systemctl enable --now banq-rtw.timer

# Verify timer status
sudo systemctl list-timers 'banq-*'
```

#### Step 8: Create Database Symlinks

After services have generated databases, create symlinks for API access:

```bash
# Example: Link databases with colon-formatted names to underscore format for API
sudo ln -sf /var/lib/banq/ri-APOW:supply:P000.db /srv/db/ri_apow_supply_0.db
sudo ln -sf /var/lib/banq/rt-XPOW_APOW:T000.db /srv/db/rt_xpow_apow_0.db
# ... (repeat for all database files)
```

**Note:** Symlinks provide access control - only symlinked databases are exposed to the API. To temporarily disable API access to a database, remove its symlink without touching the original file.

### 3. Mining Service

```bash
# Build the miner image
./docker/xpowermine/miner/build.sh

# Configure environment variables
export PROVIDER_URL=https://api.avax.network/ext/bc/C/rpc
export CONTRACT_RUN=v10a
export MINT_ADDRESS_PK=0x...  # Your private key
export MINT_ADDRESS=0x...     # Beneficiary address
export MINT_LEVEL=8
export MINE_WORKERS=7

# Run the miner
docker run --rm -ti \
  -e PROVIDER_URL="$PROVIDER_URL" \
  -e CONTRACT_RUN="$CONTRACT_RUN" \
  -e MINT_ADDRESS_PK="$MINT_ADDRESS_PK" \
  -e MINT_ADDRESS="$MINT_ADDRESS" \
  -e MINT_LEVEL="$MINT_LEVEL" \
  -e MINE_WORKERS="$MINE_WORKERS" \
  xpowermine/miner
```

See [docker/xpowermine/miner/Docker.md](docker/xpowermine/miner/Docker.md) for configuration details and FAQ.

## Configuration

### Environment Variables

**Blockchain Connection:**
- `PROVIDER_URL` - Avalanche RPC endpoint (default: `https://api.avax.network/ext/bc/C/rpc`)
- `CONTRACT_RUN` - Smart contract version (default: `v10a`)

**Mining (Miner Service Only):**
- `MINT_ADDRESS_PK` - Private key for transaction signing (required)
- `MINT_ADDRESS` - Beneficiary address for minted tokens (required)
- `MINT_LEVEL` - Proof-of-work difficulty level (default: `8`)
- `MINE_WORKERS` - Number of mining workers (default: `7`)
- `MAX_PRIORITY_FEE_PER_GAS` - Priority fee (optional, auto by default)
- `MAX_FEE_PER_GAS` - Maximum gas fee (optional, auto by default)
- `GAS_LIMIT` - Gas limit (optional, auto by default)

**API Service:**
- Command-line flags: `-p` (port), `-P` (db path), `-R` (max rows), `-O` (CORS origins)
- See [docker/xpowerbanq/banq-api/Docker.md](docker/xpowerbanq/banq-api/Docker.md) for details

### Configuration Files

- `etc/banq/banq.env.mainnet` - Mainnet blockchain configuration
- `etc/banq/banq.env.testnet` - Testnet blockchain configuration
- `etc/profile.d/banq.sh` - Shell environment setup
- `etc/nginx/sites-available/default` - Nginx reverse proxy configuration

## Production Deployment

### Nginx Reverse Proxy

For production, deploy Nginx in front of the API service:

**Key Features:**
- Rate limiting: 100 requests/second per IP (burst 200)
- Gzip compression for JSON responses (60-80% bandwidth reduction)
- SSL/TLS termination
- Keepalive connections (15-30% latency reduction)
- Upstream health monitoring

**Configuration:** See `etc/nginx/sites-available/default` for a complete production-ready setup.

### Monitoring

**API Health Check:**
```bash
curl http://localhost:8001/health
```

**Docker Stats:**
```bash
docker stats banq-api
```

**Systemd Status:**
```bash
sudo systemctl status banq-api.service
sudo systemctl list-timers 'banq-*'
sudo journalctl -u banq-api.service -f
```

### Resource Limits

**API Service:**
- CPU: 1.0 core (100%)
- Memory: 500MB
- Actual usage: ~5-10MB at idle

**Systemd Services:**
- CPU: 100% quota
- Memory: 500MB maximum

## Troubleshooting

### Permission Denied Errors

**Symptom:** Service fails with "Permission denied" when accessing `/etc/banq/banq.env.mainnet`

**Solution:**
```bash
sudo chown banq:banq /etc/banq/banq.env.mainnet
sudo chmod 640 /etc/banq/banq.env.mainnet
```

### Database Write Errors

**Symptom:** Watch services fail with "cannot create database file"

**Solution:**
```bash
# StateDirectory=banq should auto-create this, but if not:
sudo mkdir -p /var/lib/banq
sudo chown banq:banq /var/lib/banq
sudo chmod 755 /var/lib/banq
```

### Wrapper Service Failures

**Symptom:** `banq-riw.service` or `banq-rtw.service` cannot restart template services

**Solution:** Wrapper services must run as root. Do not add `User=banq` to wrapper service files.

### API Cannot Read Databases

**Symptom:** API returns "database not found" errors

**Solution:**
```bash
# Ensure symlinks exist in /srv/db
ls -la /srv/db/

# Ensure database files have correct ownership and permissions
sudo chown banq:banq /var/lib/banq/*.db* 2>/dev/null || true
sudo chmod 644 /var/lib/banq/*.db* 2>/dev/null || true

# Ensure /srv/db is readable
sudo chmod 755 /srv/db
```

## Security

### Defense-in-Depth Architecture

The security model employs multiple independent layers of isolation. Each layer provides protection even if other layers are compromised:

1. **Application Layer** - Deno permission sandbox (compiled into binary)
2. **OS Layer** - Systemd security hardening
3. **Container Layer** - Docker isolation (API service only)

### 1. Application Layer: Deno Permission Sandbox

The `banq` CLI binary is compiled with **restricted Deno permissions** that are enforced at runtime regardless of how the binary is executed.

**Permissions Granted (required for blockchain operations):**
- `--allow-env` - Read environment variables (configuration)
- `--allow-net` - Network access (Avalanche RPC endpoints)
- `--allow-read` - Read filesystem (configuration files, ABIs)
- `--allow-sys` - System information queries
- `--allow-ffi` - Foreign function interface (WASM miner, Ledger hardware wallet)

**Permissions Denied (compiled without these flags):**
- ❌ `--allow-write` - **Cannot write to filesystem**
- ❌ `--allow-run` - **Cannot execute subprocesses**
- ❌ `--allow-all` - **Not unrestricted**

**Security Implications:**

Even if the `banq` binary were compromised or exploited, an attacker is **fundamentally unable** to:
- Write malicious files to disk
- Modify existing system files or configurations
- Execute arbitrary commands or spawn shells
- Install backdoors or persist malicious code
- Escalate privileges through subprocess execution

**Note:** Database writes (e.g., `/var/lib/banq/ri-*.db`) are handled by external shell scripts (`banq-riw2db.sh`, `banq-rtw2db.sh`) that pipe the binary's stdout output. The `banq` binary itself never performs filesystem writes.

This permission model is **compiled into the binary** and cannot be bypassed without recompiling the source code.

### 2. OS Layer: Systemd Security Hardening

#### CLI Template Services (banq-ri@, banq-rt@, banq-riw@, banq-rtw@)

Comprehensive OS-level hardening (defense-in-depth on top of Deno sandbox):
- `PrivateTmp=yes` - Isolated /tmp and /var/tmp
- `NoNewPrivileges=yes` - Prevents privilege escalation
- `ProtectSystem=strict` - Read-only filesystem (except /var/lib/banq for watch services)
- `ProtectHome=yes` - Hides user home directories
- `ReadWritePaths=/var/lib/banq` - Explicit write access for database files (watch services only)
- `ProtectKernelTunables=yes` - Protects /proc/sys, /sys from writes
- `ProtectKernelModules=yes` - Prevents kernel module loading
- `ProtectKernelLogs=yes` - Denies access to kernel logs
- `ProtectControlGroups=yes` - Makes cgroup hierarchy read-only
- `RestrictRealtime=yes` - Prevents realtime scheduling
- `RestrictSUIDSGID=yes` - Prevents SUID/SGID bit changes
- `RemoveIPC=yes` - Removes IPC objects on service stop
- `PrivateMounts=yes` - Private mount namespace
- `RestrictNamespaces=yes` - Prevents namespace creation
- `LockPersonality=yes` - Locks execution domain
- `RestrictAddressFamilies=AF_INET AF_INET6` - Restricts network to IPv4/IPv6
- `SystemCallArchitectures=native` - Blocks foreign architecture syscalls

#### Wrapper Services (banq-ri, banq-riw, banq-rt, banq-rtw)

Limited hardening (constrained by systemctl/D-Bus requirements):
- `PrivateTmp=yes` - Isolated /tmp and /var/tmp
- `NoNewPrivileges=yes` - Prevents privilege escalation
- `ProtectKernelTunables=yes` - Protects /proc/sys, /sys from writes
- `ProtectKernelModules=yes` - Prevents kernel module loading
- `ProtectKernelLogs=yes` - Denies access to kernel logs
- `RestrictRealtime=yes` - Prevents realtime scheduling
- `RestrictSUIDSGID=yes` - Prevents SUID/SGID bit changes
- `LockPersonality=yes` - Locks execution domain
- `SystemCallArchitectures=native` - Blocks foreign architecture syscalls

**Note:** Cannot apply `ProtectSystem=strict` or namespace restrictions as these would prevent systemctl operations.

#### API Service (banq-api)

Systemd hardening (secondary to Docker container isolation):
- `PrivateTmp=yes` - Isolated /tmp and /var/tmp
- `NoNewPrivileges=yes` - Prevents privilege escalation
- `ProtectKernelTunables=yes` - Protects /proc/sys, /sys from writes
- `ProtectKernelModules=yes` - Prevents kernel module loading
- `ProtectKernelLogs=yes` - Denies access to kernel logs
- `RestrictRealtime=yes` - Prevents realtime scheduling
- `RestrictSUIDSGID=yes` - Prevents SUID/SGID bit changes
- `LockPersonality=yes` - Locks execution domain
- `SystemCallArchitectures=native` - Blocks foreign architecture syscalls

**Note:** Primary isolation provided by Docker container; systemd hardening is limited due to Docker management requirements.

### 3. Container Layer: Docker Isolation (API Only)

**API Service Defense-in-Depth:**
- Runs in Docker container with non-root user (`banq:banq`, UID/GID 1001)
- Container isolation provides namespace and process separation
- Read-only volume mounts (`/var/lib/banq:ro`, `/srv/db:ro`)
- Resource limits (1.0 CPU, 500MB memory)
- Network exposure limited to localhost only (`127.0.0.1:8001`)
- Built-in health monitoring
- Hardcoded SQL queries (no dynamic SQL)
- Input validation and sanitization
- CORS origin whitelisting

### Privilege Separation Model

The architecture employs privilege separation to minimize security risk:

```
┌─────────────────────────────────────────┐
│   Wrapper Services (root)               │
│   - banq-riw.service                    │
│   - banq-rtw.service                    │
│   - Execute systemctl commands          │
└─────────────────────────────────────────┘
                  ↓ restart
┌─────────────────────────────────────────┐
│   Template Services (banq:banq)         │
│   - banq-ri@.service                    │
│   - banq-riw@.service                   │
│   - banq-rt@.service                    │
│   - banq-rtw@.service                   │
│   - Execute /usr/local/bin/banq         │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│   Deno Permission Sandbox               │
│   - No filesystem writes (--allow-write)│
│   - No subprocess exec (--allow-run)    │
└─────────────────────────────────────────┘
```

**Why Template Services Run as banq:banq:**
1. **Least Privilege** - Services only need to read blockchain data and write to `/var/lib/banq`
2. **Isolation** - Compromised service cannot affect system-wide resources
3. **Defense-in-Depth** - Additional layer on top of Deno's permission sandbox and systemd hardening

**Why Wrapper Services Run as root:**
- Wrapper services execute `systemctl restart` commands to control template service instances
- This requires root privileges (or complex polkit configuration)
- Running wrappers as root is the minimal-change approach that:
  - Avoids complex polkit rule setup
  - Maintains existing architecture
  - Isolates privileged operations to trusted wrapper scripts only

**Service Privilege Summary:**

**API Service (`banq-api.service`):**
- **Only externally-facing service** (localhost-only binding)
- Runs in Docker container with non-root user (`banq:banq`)
- Systemd service itself runs as root (standard for Docker management)
- Defense-in-depth: Container + Systemd + Application hardening

**CLI-Based Template Services:**
- **Trusted internal automation** (no external exposure)
- Execute Deno-compiled `banq` binary (sandboxed)
- Run as dedicated `banq:banq` user (non-root)
- Process local blockchain data only (no external input)
- Defense-in-depth: Deno sandbox + non-root user + systemd hardening

**Wrapper Services:**
- **Systemd orchestrators** (no external exposure)
- Run as root to execute `systemctl restart` commands
- Execute trusted shell scripts only
- Minimal privileged surface (only restart child services)

### Security Posture Summary

**Attack Surface:**
- **External:** Only `banq-api.service` (localhost-only, Docker-isolated, non-root user)
- **Internal:** CLI automation services (Deno-sandboxed, trusted code, no external input)

**Layered Protections:**
1. **Deno Permission Sandbox** (Application Layer)
   - Prevents filesystem writes by `banq` binary
   - Prevents subprocess execution by `banq` binary
   - Enforced regardless of systemd or user context

2. **Systemd Hardening** (OS Layer)
   - Filesystem isolation and kernel protection
   - Namespace and resource restrictions
   - Defense-in-depth on top of Deno sandbox

3. **Docker Isolation** (Container Layer - API only)
   - Non-root user inside container
   - Read-only volume mounts
   - Localhost-only network binding

**Threat Model Assessment:**

The configuration provides robust protection against:
- ✅ External attacks (single hardened API service, localhost-only)
- ✅ Container escape attempts (multiple isolation layers)
- ✅ Filesystem tampering (Deno `--allow-write` not granted + systemd `ProtectSystem=strict`)
- ✅ Privilege escalation (Deno `--allow-run` not granted + systemd `NoNewPrivileges=yes`)
- ✅ Malicious code persistence (no write capabilities in banq binary)
- ✅ Resource exhaustion (memory/CPU limits across all services)
- ✅ Kernel exploitation (systemd kernel protections + Docker namespace isolation)

**Design Philosophy:**

Security is context-appropriate:
- **External-facing service** (API): Maximum isolation via Docker + non-root user + localhost binding
- **Internal automation** (CLI): Deno sandbox prevents filesystem writes/process execution; systemd hardening provides defense-in-depth
- **Orchestration** (wrappers): Trusted code path executing known shell scripts

## Technology Stack

### Backend
- **Go 1.21** - API service (banq-api)
- **Deno** - CLI tools and miner
- **SQLite 3** - Embedded database (WAL mode)
- **Bash** - Orchestration scripts

### Web Infrastructure
- **Chi v5** - HTTP router (radix tree)
- **Nginx** - Reverse proxy, rate limiting, SSL
- **CORS** - Cross-origin support

### Infrastructure
- **Docker** - Containerization
- **Systemd** - Service management, scheduling
- **Supercronic** - Container cron scheduler

### Blockchain
- **Avalanche C-Chain** - Smart contract platform
- **Web3/Ethers.js** - Blockchain interaction (via Deno)

## Database Schema

### Rate Index Databases (ri_*.db)

```sql
CREATE TABLE raw_logs (
  id TEXT PRIMARY KEY,
  json TEXT
);

CREATE VIEW riw_view AS
  SELECT
    json_extract(json, '$.util_e18') AS util_e18,
    json_extract(json, '$.stamp_iso') AS stamp_iso
  FROM raw_logs;

CREATE INDEX idx_block_number ON raw_logs(json_extract(json, '$.block_number'));
CREATE INDEX idx_stamp ON raw_logs(json_extract(json, '$.stamp'));
```

### Rate Tracker Databases (rt_*.db)

```sql
CREATE VIEW rtw_view AS
  SELECT
    json_extract(json, '$.quote_bid_e18') AS quote_bid_e18,
    json_extract(json, '$.quote_ask_e18') AS quote_ask_e18,
    json_extract(json, '$.quote_time_iso') AS quote_time_iso
  FROM raw_logs;
```

## SQLite Configuration and Tuning

### Current Configuration

The databases are already optimized for concurrent read/write operations:

**Write-Ahead Logging (WAL):**
- All databases use WAL mode for concurrent access
- Allows readers to access the database while writes are in progress
- Configured automatically by the ingestion scripts

**Connection Pooling (API Service):**
- 20 maximum connections per database
- 10 idle connections maintained
- Defined in `docker/xpowerbanq/banq-api/source/database.go`

**Batched Writes (Ingestion Scripts):**
- Default batch size: 16 rows per transaction
- Configurable via `DB_PAGE` in `banq-riw2db.sh` and `banq-rtw2db.sh`
- Short transactions prevent blocking API readers

### Performance Tuning Options

#### Write-Side Optimization (Ingestion Scripts)

For better write performance during blockchain indexing, these PRAGMA settings can be applied:

```bash
# In banq-riw2db.sh or banq-rtw2db.sh, after opening the database:
sqlite3 "$DB_PATH" <<EOF
PRAGMA journal_mode=WAL;           -- Already enabled
PRAGMA synchronous=NORMAL;         -- Faster writes (safe with WAL)
PRAGMA cache_size=-64000;          -- 64MB cache (negative = KB)
PRAGMA temp_store=MEMORY;          -- Temp tables in memory
PRAGMA mmap_size=268435456;        -- 256MB memory-mapped I/O
EOF
```

**Trade-offs:**
- `synchronous=NORMAL` - Faster writes, minimal durability risk (safe with WAL)
- `cache_size=-64000` - More memory usage, fewer disk reads
- `mmap_size=268435456` - Faster reads/writes, requires sufficient RAM

**Not Recommended:**
- `synchronous=OFF` - Risks database corruption on system crash
- Disabling WAL mode - Breaks concurrent read/write capability

#### Read-Side Optimization (API Service)

The Go API opens databases in **read-only mode** (`file:path?mode=ro`), which:
- Prevents accidental writes
- Enables safe concurrent access across multiple connections
- Allows multiple processes to share page cache

**Current Settings (already optimized):**
- Read-only mode (`mode=ro`)
- Connection pooling (20 max, 10 idle)
- Prepared statements cached per connection

**Additional Tuning (if needed):**

To apply read-side optimizations, modify `database.go` to execute PRAGMA statements after opening connections:

```go
// After opening the database connection:
db.Exec("PRAGMA cache_size=-32000")      // 32MB cache per connection
db.Exec("PRAGMA mmap_size=134217728")    // 128MB memory-mapped I/O
db.Exec("PRAGMA query_only=ON")          // Extra safety
```

**Verification:**
```bash
# Check current PRAGMA settings
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "PRAGMA journal_mode; PRAGMA synchronous;"
# Should output: wal, 2 (FULL) or 1 (NORMAL)

# Check database size
du -h /var/lib/banq/*.db

# Monitor database file usage
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "PRAGMA page_count; PRAGMA page_size; PRAGMA freelist_count;"
```

### Batch Size Tuning

The ingestion scripts commit every `DB_PAGE` rows (default: 16). Adjust based on workload:

**Smaller batches (8-12 rows):**
- Pros: Less reader blocking, better concurrency
- Cons: More fsync overhead, slightly slower writes

**Larger batches (32-64 rows):**
- Pros: Faster bulk writes, less fsync overhead
- Cons: Longer transactions may block readers

**Recommended:** Keep default (16 rows) unless profiling shows bottlenecks.

### Performance Monitoring

**Check WAL checkpoint status:**
```bash
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "PRAGMA wal_checkpoint(PASSIVE);"
```

**Monitor WAL file size:**
```bash
ls -lh /var/lib/banq/*.db-wal
# Large WAL files (>10MB) indicate checkpoint backlog
```

**Force checkpoint (if needed):**
```bash
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "PRAGMA wal_checkpoint(TRUNCATE);"
```

**Query performance analysis:**
```bash
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db <<EOF
EXPLAIN QUERY PLAN
SELECT AVG(json_extract(json, '$.util_e18')) FROM raw_logs
WHERE DATE(json_extract(json, '$.stamp_iso')) BETWEEN '2025-01-01' AND '2025-01-31';
EOF
```

### Disk Space Management

**Database sizes (approximate):**
- Each RI database: 50-500MB (depends on activity)
- Each RT database: 20-200MB
- WAL files: 1-10MB (transient)

**Total storage estimate:** 15-30GB for all 42 databases

**Cleanup (if needed):**
```bash
# Vacuum to reclaim space (run during low-traffic periods)
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "VACUUM;"

# Auto-vacuum (set at database creation, not recommended for write-heavy workloads)
# PRAGMA auto_vacuum=INCREMENTAL;
```

## Development

### Running Tests (API Service)

```bash
cd docker/xpowerbanq/banq-api

# Run all tests
make test

# Run with coverage
make test-coverage

# Generate HTML coverage report
make coverage
```

### Building from Source

**API Service:**
```bash
cd docker/xpowerbanq/banq-api
go build -o banq-api ./source/
```

**Docker Images:**
```bash
# API
./docker/xpowerbanq/banq-api/build.sh

# Miner
./docker/xpowermine/miner/build.sh
```

## Documentation

- [XPower Banq API Guide](docker/xpowerbanq/banq-api/Docker.md) - Complete guide (deployment, development, testing)
- [XPowerMiner Guide](docker/xpowermine/miner/Docker.md) - Mining setup, configuration, FAQ

## Support

For issues, questions, or contributions, please refer to the project repository.

## License

GPL-3.0 - See project repository for details.
