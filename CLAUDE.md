# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a production server infrastructure for the XPower Banq DeFi protocol on Avalanche. It consists of:

1. **Go REST API** - Read-only access to utilization rates and price quotes (Chi router, SQLite)
2. **Systemd Services** - Scheduled blockchain indexing and TWAP calculation (Deno CLI, Bash orchestration)
3. **Docker Services** - Containerized API and cryptocurrency miner

The repository follows Unix Filesystem Hierarchy Standard (FHS) with directories mapping to system paths: `etc/`, `usr/`, `var/`.

## Common Commands

### API Service (Go)

```bash
# Build Docker image
./docker/xpowerbanq/banq-api/build.sh

# Build from source (native binary)
cd docker/xpowerbanq/banq-api
go build -o banq-api ./source/

# Run all tests
make test

# Run tests with coverage
make test-coverage

# Generate HTML coverage report
make coverage

# Run specific test
make test-run TEST=SecuritySQLInjection

# Run tests with verbose output
make test-verbose

# Run container locally
docker run -d --name banq-api \
  --cpus=1.0 --memory=500m --memory-swap=500m \
  -v /var/lib/banq:/var/lib/banq:rw \
  -v /srv/db:/srv/db:ro \
  -p 8001:8001 \
  xpowerbanq/banq-api

# Test API endpoint
curl http://localhost:8001/health
curl "http://localhost:8001/ri_apow_supply_0/daily_average?lhs=2025-11-15&rhs=2025-12-15"
```

### Miner Service (Deno)

```bash
# Build Docker image
./docker/xpowermine/miner/build.sh

# Run miner
docker run --rm -ti \
  -e PROVIDER_URL="$PROVIDER_URL" \
  -e CONTRACT_RUN="$CONTRACT_RUN" \
  -e MINT_ADDRESS_PK="$MINT_ADDRESS_PK" \
  -e MINT_ADDRESS="$MINT_ADDRESS" \
  -e MINT_LEVEL="$MINT_LEVEL" \
  -e MINE_WORKERS="$MINE_WORKERS" \
  xpowermine/miner
```

### Systemd Services

```bash
# Install systemd services (requires root)
sudo cp etc/systemd/system/*.service /etc/systemd/system/
sudo cp etc/systemd/system/*.timer /etc/systemd/system/
sudo systemctl daemon-reload

# Enable and start services
sudo systemctl enable --now banq-api.service
sudo systemctl enable --now banq-ri.timer
sudo systemctl enable --now banq-riw.timer
sudo systemctl enable --now banq-rt.timer
sudo systemctl enable --now banq-rtw.timer

# Check service status
sudo systemctl status banq-api.service
sudo systemctl list-timers 'banq-*'
sudo journalctl -u banq-api.service -f

# Manually trigger wrapper service (orchestrates all template instances)
sudo systemctl start banq-riw.service
```

## Architecture

### Multi-Layered Request Flow

```
Client → Nginx (rate limit, gzip, SSL) → Docker API (Chi router, port 8001) → SQLite (/srv/db/*.db)
                                                                                      ↑
                                              Systemd Services (timers) → banq CLI → Blockchain
```

### Key Architectural Patterns

#### 1. Systemd Template Services with Parameter Parsing

**Pattern:** `banq-{function}@{PARAM1}_{PARAM2}_{PARAM3}.service` or `banq-{function}@{PARAM1}:{PARAM2}:{PARAM3}.service`

**Parameter Extraction in Systemd Units:**
```systemd
[Service]
ExecStart=/usr/bin/bash -c 'IFS=_: read T M P <<< "%I"; /usr/local/bin/banq reindex ...'
```

The `%I` placeholder is replaced with the instance name (e.g., `APOW:supply:P000` or `APOW_supply_P000`), then parsed into variables using bash's `read` with `IFS=_:` as delimiters (both formats work).

**Examples:**
- `banq-ri@APOW:supply:P000.service` → `T=APOW, M=supply, P=P000`
- `banq-riw@APOW:supply:P000.service` → `T=APOW, M=supply, P=P000`
- `banq-rt@XPOW_APOW:T000.service` → `T0=XPOW, T1=APOW, O=T000`
- `banq-rtw@XPOW_APOW:T000.service` → `T0=XPOW, T1=APOW, O=T000`

**Wrapper Services:**
- `banq-riw.sh` orchestrates 28 RI instances (4 pools × 7 tokens × 2 modes: supply/borrow)
- `banq-rtw.sh` orchestrates 14 RT instances (7 token pairs × 2 directions)
- Executed by wrapper services (e.g., `banq-riw.service`) triggered by systemd timers

#### 2. Data Ingestion Pipeline

**Flow:** Blockchain → Deno CLI (stdout JSON) → Bash pipe → SQLite batch insert

**Critical Pattern in `banq-riw2db.sh`:**
```bash
# Long-lived SQLite connection over FD 3 (avoids pipe overhead)
exec 3> >(sqlite3 "$DB_PATH" >/dev/null 2>&1)

# Batch insert every 16 rows (configurable)
printf 'BEGIN;\n' >&3
while IFS= read -r line; do
  # ... INSERT OR REPLACE ...
  if (( n % DB_PAGE == 0 )); then
    printf 'COMMIT; BEGIN;\n' >&3  # Short transactions keep readers unblocked
  fi
done
printf 'COMMIT;\n' >&3
exec 3>&-
```

**Why this matters:**
- WAL mode allows concurrent reads during writes
- Batching reduces fsync overhead
- Short transactions prevent blocking the API service

#### 3. Chi Router with Dynamic Route Registration

**Pattern in `main.go` + `handlers.go`:**
```go
// config.go: Route definitions map endpoint → SQL + scanner
var endpointRoutes = map[string]*RouteConfig{
    "/daily_average.json": {
        DBPrefix: "ri_",
        SQL: dailyAverageSQL,  // Hardcoded, parameterized query
        ResultScanner: scanDailyAverage,
    },
}

// handlers.go: Dynamic route registration
func registerAPIRoutes(r *chi.Mux) {
    for endpoint, cfg := range endpointRoutes {
        r.Get("/{dbName}" + endpoint, makeHandler(cfg))
    }
}
```

**Why this matters:**
- Adding a new endpoint only requires modifying `endpointRoutes` map
- SQL is hardcoded (prevents injection)
- Database name prefix validates allowed databases (e.g., `ri_*`, `rt_*`)

### Security Architecture (Defense-in-Depth)

**Three Independent Layers:**

1. **Application Layer:** Deno Permission Sandbox
   - Compiled into `banq` binary: `--allow-write` DENIED, `--allow-run` DENIED
   - Cannot write files or execute subprocesses even if compromised

2. **OS Layer:** Systemd Hardening
   - `ProtectSystem=strict` (read-only filesystem except `/var/lib/banq`)
   - `NoNewPrivileges=yes` (prevents privilege escalation)
   - `RestrictNamespaces=yes` (blocks container creation)

3. **Container Layer:** Docker Isolation (API only)
   - Non-root user (`banq:banq`, UID 1001)
   - Read-only volume mounts (`:ro`)
   - Localhost-only binding (`127.0.0.1:8001`)

**Critical Security Pattern:** Database writes are handled by **external shell scripts** (`banq-riw2db.sh`) that pipe `banq` CLI stdout. The Deno binary itself never writes to the filesystem.

### Database Schema Pattern

**Design:** JSON storage + indexed views + hardcoded SQL

```sql
-- Raw storage (minimal schema)
CREATE TABLE raw_logs (id TEXT PRIMARY KEY, json TEXT);

-- Extracted view (query optimization)
CREATE VIEW riw_view AS
  SELECT
    json_extract(json, '$.util_e18') AS util_e18,
    json_extract(json, '$.stamp_iso') AS stamp_iso
  FROM raw_logs;

-- Indexes on JSON fields
CREATE INDEX idx_block_number ON raw_logs(json_extract(json, '$.log.blockNumber'));
```

**Why this matters:**
- Flexible schema evolution (just add JSON fields)
- Indexes on JSON extracts enable fast queries
- Views simplify API queries while maintaining raw data

## Project Structure (Key Files)

```
docker/xpowerbanq/banq-api/source/
├── main.go          # Chi router setup, CORS middleware, startup
├── config.go        # Route definitions, SQL queries, configuration globals
├── handlers.go      # HTTP handlers, dynamic route registration
├── database.go      # Connection pooling (20 max, 10 idle per DB)
├── parameters.go    # Query parameter validation (date regex: YYYY-MM-DD)
├── scanners.go      # Database row scanners for each endpoint
├── args.go          # CLI argument parsing (--port, --db-path, --max-rows, --cors-origins)
└── types.go         # Structs: RouteConfig, DailyAverage, DailyOHLC

usr/local/bin/
├── banq-riw.sh      # Wrapper: restart all banq-riw@*.service instances
├── banq-riw2db.sh   # Pipe JSON from stdin → SQLite batched INSERT
├── banq-rtw.sh      # Wrapper: restart all banq-rtw@*.service instances
└── banq-rtw2db.sh   # Pipe TWAP JSON → SQLite batched INSERT

etc/systemd/system/
├── banq-riw@.service       # Template: blockchain indexing (watch mode)
├── banq-riw.service        # Wrapper: calls banq-riw.sh
├── banq-riw.timer          # Schedule: twice daily (06:45, 18:45)
├── banq-rt@.service        # Template: TWAP calculation
├── banq-rtw@.service       # Template: TWAP calculation (watch mode)
└── banq-api.service        # Docker container management
```

## Development Patterns

### Adding a New API Endpoint

1. **Add SQL query to `config.go`:**
   ```go
   myNewSQL = `SELECT ... FROM view WHERE ... LIMIT ?`
   ```

2. **Create scanner function in `scanners.go`:**
   ```go
   func scanMyNew(rows *sql.Rows) ([]MyNewType, error) { ... }
   ```

3. **Define type in `types.go`:**
   ```go
   type MyNewType struct { Field1 string `json:"field1"` }
   ```

4. **Register route in `config.go` `endpointRoutes` map:**
   ```go
   "/my_new.json": {
       DBPrefix: "ri_",
       SQL: myNewSQL,
       ResultScanner: scanMyNew,
   }
   ```

5. **Write test in `handlers_test.go`**

### Adding a New Systemd Service Instance

**For Rate Index (new pool P007 for AVAX):**
```bash
# Add to usr/local/bin/banq-riw.sh:
systemctl restart "banq-riw@AVAX:supply:P007.service"
systemctl restart "banq-riw@AVAX:borrow:P007.service"

# The template service (banq-riw@.service) handles parameter extraction automatically
```

**For Rate Tracker (new oracle T007 for AVAX/USDC):**
```bash
# Add to usr/local/bin/banq-rtw.sh:
systemctl restart "banq-rtw@AVAX:USDC:T007.service"
```

### Testing Locally Without Systemd

```bash
# Test database ingestion pipeline
echo '{"id":"test1","util_wad":"1000000000000000000n","stamp":"1700000000n","log":{...}}' \
  | /usr/local/bin/banq-riw2db.sh /tmp/test-ri.db 16

# Verify data
sqlite3 /tmp/test-ri.db "SELECT * FROM riw_view LIMIT 5;"

# Test API with local database
docker run -d --name test-api \
  -v /tmp:/tmp:ro \
  -p 8001:8001 \
  xpowerbanq/banq-api \
  -P /tmp -p 8001
```

## Important Constraints

### Go API Service

- **All SQL must be hardcoded in `config.go`** - Never construct SQL dynamically
- **Database names must match prefix** - `DBPrefix` field validates allowed databases
- **Date format is strict** - `YYYY-MM-DD` validated via regex in `parameters.go`
- **Connection pooling is pre-configured** - 20 max, 10 idle per database (see `database.go`)
- **CORS origins are whitelisted** - Modify `allowedOrigins` map in `config.go`, not middleware
- **Non-root container user** - Dockerfile creates `banq:banq` (UID/GID 1001), do not change

### Systemd Services

- **2-second startup delay is required** - Prevents nonce conflicts when multiple instances start (template services: `ExecStartPre=/usr/bin/sleep 2`)
- **Wrapper services must orchestrate all instances** - Timers trigger wrappers, not individual templates
- **Database writes only in `*2db.sh` scripts** - Deno CLI cannot write files (permission denied)
- **Batch size affects concurrency** - Default 16 rows (`DB_PAGE=16` in `banq-riw2db.sh`); smaller = more COMMITs = less reader blocking

### Deno Binary

- **No filesystem writes** - Compiled without `--allow-write`, cannot be changed without recompiling
- **No subprocess execution** - Compiled without `--allow-run`
- **Outputs JSON to stdout only** - Ingestion scripts must pipe output

## Configuration Files

- **`etc/banq/banq.env.mainnet`** - Blockchain connection (3 variables: `PROVIDER_URL`, `CONTRACT_RUN`, `PRIVATE_KEY`)
- **`etc/profile.d/banq.sh`** - Shell environment setup (sources `banq.env.mainnet`)
- **`docker/xpowerbanq/banq-api/go.mod`** - Go dependencies (Chi v5, CORS, SQLite3)
- **`docker/xpowerbanq/banq-api/Makefile`** - Test targets (`test`, `test-coverage`, `coverage`, `test-run`, `test-verbose`)

## Monitoring and Debugging

```bash
# Check systemd timer schedule
sudo systemctl list-timers 'banq-*'

# View logs for specific service
sudo journalctl -u banq-riw@APOW:supply:P000.service -f

# Check database ingestion (note: actual database uses colon format)
sqlite3 /var/lib/banq/ri-APOW:supply:P000.db "SELECT COUNT(*) FROM raw_logs;"

# Verify API health
curl http://localhost:8001/health

# Check Docker container stats
docker stats banq-api

# Test database query directly
sqlite3 /srv/db/ri_apow_supply_0.db <<EOF
PRAGMA journal_mode;  -- Should output: wal
SELECT COUNT(*) FROM riw_view;
EOF
```

## Common Issues

**Systemd service fails with "Cannot create /var/lib/banq/*.db":**
- Check directory permissions: `sudo chown -R root:root /var/lib/banq`
- Verify `ReadWritePaths=/var/lib/banq` in service file (watch services only)

**API returns "database not found":**
- Database files are stored with colon format: `/var/lib/banq/ri-TOKEN:MODE:POOL.db` (e.g., `ri-APOW:supply:P000.db`)
- API expects underscore format symlinks: `/srv/db/ri_token_mode_pool.db` (e.g., `/srv/db/ri_apow_supply_0.db`)
- Check symlinks exist: `ls -la /srv/db/` should show symlinks pointing to `/var/lib/banq/*.db`
- Verify Docker mount: `-v /srv/db:/srv/db:ro`
- Example symlink: `sudo ln -sf /var/lib/banq/ri-APOW:supply:P000.db /srv/db/ri_apow_supply_0.db`

**Test coverage < 50%:**
- Run `make coverage` to generate HTML report
- Focus on `handlers_test.go` and `security_test.go`
- New endpoints require corresponding test cases

**Rate limiting in Nginx:**
- Check `limit_req_zone` in `etc/nginx/sites-available/default`
- Default: 100 req/s per IP, burst 200
- Increase `rate=100r/s` if needed for production
