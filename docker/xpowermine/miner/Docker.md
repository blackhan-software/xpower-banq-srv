# XPower Miner - Docker Deployment Guide

> **WARNING:** Mining can seriously damage your device due to intensive CPU usage. Use at your own risk.

Containerized Deno-based cryptocurrency mining service for XPower tokens on the Avalanche blockchain.

## Overview

This Docker container runs the XPower cryptocurrency miner, which:

- Mines XPower tokens on the Avalanche C-Chain
- Uses multiple workers for parallel proof-of-work computation
- Automatically or manually configures gas parameters
- Integrates with Avalanche smart contracts (v10a by default)

**Technology Stack:**
- Deno runtime (Alpine-based)
- Supercronic scheduler
- Avalanche blockchain integration

## Quick Start

### Build

```sh
./docker/xpowermine/miner/build.sh
```

Or directly:

```sh
docker build -t xpowermine/miner -f docker/xpowermine/miner/Dockerfile .
```

## Configuration

### Required Environment Variables

**Blockchain Configuration:**

```sh
CONTRACT_RUN=v10a # Required: Smart contract version (v10a, v10b, etc.)
```

**Minting Configuration:**

```sh
MINT_ADDRESS_PK=0x... # Required: Private key for transaction signing
MINT_ADDRESS=0x...    # Required: Beneficiary address for minted tokens
```

### Optional Environment Variables

**Blockchain Connection:**

```sh
PROVIDER_URL=https://api.avax.network/ext/bc/C/rpc # Default: Avalanche public RPC
```

**Mining Parameters:**

```sh
MINT_LEVEL=8   # Default: 8 (proof-of-work difficulty, 2^8-1=255 XPOW minimum)
MINE_WORKERS=7 # Default: 7 (number of mining workers, set to cores-1)
```

**Gas Parameters (Optional):**

```sh
MAX_PRIORITY_FEE_PER_GAS=0 # Default: auto (0 recommended to avoid extra fees)
MAX_FEE_PER_GAS=500000000  # Default: auto (0.5 Gwei recommended)
GAS_LIMIT=100000           # Default: auto (0.1 Mwei recommended)
```

## Running the Container

### With Automatic Gas Parameters (Recommended)

```sh
docker run --rm -ti \
-e PROVIDER_URL="$PROVIDER_URL" \
-e CONTRACT_RUN="$CONTRACT_RUN" \
-e MINT_ADDRESS_PK="$MINT_ADDRESS_PK" \
-e MINT_ADDRESS="$MINT_ADDRESS" \
-e MINT_LEVEL="$MINT_LEVEL" \
-e MINE_WORKERS="$MINE_WORKERS" \
xpowermine/miner
```

### With Custom Gas Parameters

```sh
# Set custom gas parameters
export MAX_PRIORITY_FEE_PER_GAS=0 # 0.0 gwei
export MAX_FEE_PER_GAS=500000000  # 0.5 gwei
export GAS_LIMIT=100000           # 0.1 mwei

# Run with custom gas settings
docker run --rm -ti \
-e PROVIDER_URL="$PROVIDER_URL" \
-e CONTRACT_RUN="$CONTRACT_RUN" \
-e MINT_ADDRESS_PK="$MINT_ADDRESS_PK" \
-e MINT_ADDRESS="$MINT_ADDRESS" \
-e MINT_LEVEL="$MINT_LEVEL" \
-e MINE_WORKERS="$MINE_WORKERS" \
-e MAX_PRIORITY_FEE_PER_GAS="$MAX_PRIORITY_FEE_PER_GAS" \
-e MAX_FEE_PER_GAS="$MAX_FEE_PER_GAS" \
-e GAS_LIMIT="$GAS_LIMIT" \
xpowermine/miner
```

## Configuration Guide

### PROVIDER_URL

**Default:** `https://api.avax.network/ext/bc/C/rpc`

The public RPC endpoint of the Avalanche network works well most of the time. However, if your container is behind a popular VPN or your minting frequency is high, using your own custom endpoint is recommended.

**Recommendation:** Use the default unless you experience rate limiting or connectivity issues.

### MINT_ADDRESS_PK (Required)

The private key of an address that holds AVAX to pay for minting transaction costs.

**Security Recommendations:**
- Do NOT fund this address with large amounts of AVAX
- Regularly refill with small amounts instead
- Keep the private key secure and never commit it to version control
- Consider using a dedicated address only for mining operations

### MINT_ADDRESS (Required)

The beneficiary address that will receive the mined and minted XPOW tokens.

**Security Recommendations:**
- Use a DIFFERENT address from `MINT_ADDRESS_PK` for operational security
- This address can be your main wallet or a dedicated receiving address
- Does not need to hold any AVAX

### MINT_LEVEL

**Default:** `8`

Controls the minimum proof-of-work difficulty. Only work corresponding to `2^MINT_LEVEL - 1` XPOW or more will be minted.

**Trade-offs:**
- **Lower values (e.g., 6-7):** Faster mining, more frequent mints, higher total gas costs
- **Higher values (e.g., 9-10):** Slower mining, less frequent mints, lower total gas costs, higher rewards per mint

**Recommendation:** `8` or higher for current state-of-the-art CPUs. This provides a good balance between energy consumption and minting costs.

### MINE_WORKERS

**Default:** `7`

Number of parallel mining workers (threads) to use.

**Recommendation:** Set to one LESS than your total CPU cores to avoid system lockup.

**Examples:**
- 4-core CPU: `MINE_WORKERS=3`
- 8-core CPU: `MINE_WORKERS=7`
- 16-core CPU: `MINE_WORKERS=15`

Check your CPU count: `nproc` or `lscpu`

### MAX_PRIORITY_FEE_PER_GAS

**Default:** Auto (blockchain determines)

Priority fee paid to validators for transaction inclusion.

**Recommendation:** `0` (zero) to avoid paying extra fees. This seems to work well on Avalanche. Increase to `1000000000` (1 Gwei) only if transactions are regularly rejected.

### MAX_FEE_PER_GAS

**Default:** Auto (blockchain determines)

Maximum total gas fee willing to pay per transaction.

**Recommendation:** `500000000` (0.5 Gwei) or less. Increase to `1000000000` (1 Gwei) only if transactions are regularly rejected.

### GAS_LIMIT

**Default:** Auto (blockchain determines)

Maximum gas units to use for each transaction.

**Recommendation:** `100000` (0.1 Mwei). This value seems to work reliably for minting operations.

## Security Considerations

**Private Key Management:**
- Never commit `MINT_ADDRESS_PK` to version control
- Use environment variables or secure secrets management
- Keep minimal AVAX in the minting address
- Use a separate beneficiary address (`MINT_ADDRESS`)

**Resource Management:**
- Mining is CPU-intensive and can damage hardware over time
- Monitor CPU temperatures and usage
- Ensure adequate cooling
- Consider electricity costs vs mining rewards

**Network Security:**
- The container only makes outbound connections to the blockchain
- No ports need to be exposed
- All sensitive data is passed via environment variables

## Monitoring

**Container Logs:**
```sh
docker logs -f xpowermine-miner
```

**Resource Usage:**
```sh
docker stats xpowermine-miner
```

**System Temperature (if available):**
```sh
sensors # Linux with lm-sensors
```

## Troubleshooting

**Transactions rejected:**
- Increase `MAX_FEE_PER_GAS` and/or `MAX_PRIORITY_FEE_PER_GAS`
- Check AVAX balance in `MINT_ADDRESS_PK` address
- Verify `PROVIDER_URL` is accessible

**Low mining performance:**
- Increase `MINE_WORKERS` (but leave at least 1 core free)
- Verify CPU is not thermal throttling
- Check if other processes are consuming CPU

**Container exits:**
- Check logs: `docker logs xpowermine-miner`
- Verify all required environment variables are set
- Ensure `MINT_ADDRESS_PK` and `CONTRACT_RUN` are correct

## License

Apache-2.0 - See project repository for details.
