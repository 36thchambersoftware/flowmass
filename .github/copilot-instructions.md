# Flowmass — Copilot instructions for AI coding agents

## Overview

Flowmass is a Go-based Cardano NFT minting engine that automatically mints NFTs when a monitored address receives a 27 ADA (27,000,000 lovelace) deposit. Uses `cardano-cli` for all blockchain operations.

**Core premise:** No Discord bot. Pure Cardano automation.

## What You Should Know to Be Productive

### Minting Flow
1. **Monitor Address**: Engine polls for 27 ADA deposits (via Blockfrost API or local mock file).
2. **Deposit Detection**: Unprocessed deposits are fetched; processed ones marked to avoid duplicates.
3. **State Tracking**: Mint counter increments (nft1, nft2, etc); processed tx hashes stored in `flowmass.state`.
4. **cardano-cli Integration**: Build → Sign → Submit workflow for minting and NFT transfer.

### Key Configuration
- `MONITOR_ADDRESS` – Address to watch for incoming 27 ADA
- `POLICY_ID` – NFT minting policy ID
- `SCRIPT_FILE` – Path to minting script (e.g., `policy.script`)
- `METADATA_FILE` – Path to metadata template JSON
- `STATE_FILE` – File tracking next mint ID and processed deposits (default: `flowmass.state`)
- `BLOCKFROST_API_KEY` / `BLOCKFROST_NETWORK` – Optional; enables on-chain deposit polling

### Deposit Sources
1. **Blockfrost** (if `BLOCKFROST_API_KEY` set): Queries `/addresses/{address}/utxos` and filters for 27 ADA UTxOs.
2. **Mock File** (fallback): Reads `mock_deposits.json` for testing. Format:
   ```json
   [
     {"monitor": "addr1...", "sender": "addr1...", "amount": 27000000, "tx": "tx_hash"}
   ]
   ```

## Important Files & Entry Points

- **main.go** – Parses flags, initializes engine, starts polling loop.
- **engine.go** – Deposit polling (`pollDeposits`), orchestrates minting (`mintNFTForDeposit`).
  - `Start()` – Polling loop (30s ticker).
  - `Stop()` – Graceful shutdown.
  - `fetchDeposits()` – Routes to Blockfrost or mock.
- **state.go** – Persistent state management.
  - `LoadState()` – Loads from file or initializes.
  - `NextMintID()` – Returns incremented counter (thread-safe).
  - `MarkProcessed(txHash)` – Marks deposit as processed.
  - `Save()` – Persists to file.
- **cardano.go** – `cardano-cli` command wrappers (mostly stubs).
  - `GetCurrentSlot()` – Query tip for current slot.
  - `BuildTransaction()`, `SignTransaction()`, `SubmitTransaction()` – TX lifecycle.
  - `GetUTxOs()`, `SendNFT()` – UTXO queries and transfers.

## Patterns & Conventions

### State Persistence
- **In-memory Map + File Persistence**: `State.processedSet` (map) for fast lookups; JSON file for durability.
- **Thread-safe**: Uses `sync.Mutex` for state updates.

### Deposit Processing
- **Idempotent**: Same tx hash processed only once (checked via `IsProcessed()`).
- **Async**: Polling runs in goroutine; non-blocking.

### cardano-cli Wrapper Stubs
Functions in `cardano.go` are skeleton implementations. Expect to:
- Parse JSON outputs from `cardano-cli query tip`
- Build correct CLI argument lists for transactions
- Handle errors and retries

### Mock-First Testing
- Create `mock_deposits.json` for offline testing
- No Blockfrost key? Engine falls back to mock file automatically

## Concrete Examples & Edit Targets

### Adding Real Blockfrost UTxO Resolution
- **Current**: Sender address is "unknown" when fetched from Blockfrost
- **Fix**: `engine.go:fetchDepositsBlockfrost()` — add logic to query full transaction to resolve input sender

### Completing Transaction Building
- **Current**: `cardano.go:BuildTransaction()` returns stubs
- **Fix**: Implement full `cardano-cli transaction build` call with correct arguments:
  ```bash
  cardano-cli transaction build \
    --babbage-era \
    --testnet-magic 2 \
    --tx-in <UTXO> \
    --mint "1 <POLICY_ID>.nft<ID>" \
    --minting-script-file <SCRIPT> \
    --tx-out <RECIPIENT_ADDR>+<CHANGE> \
    --invalid-hereafter <SLOT+10000> \
    --metadata-json-file <METADATA> \
    --out-file tx.raw
  ```

### Implementing Full Minting Workflow
- **Current**: `engine.go:mintNFTForDeposit()` is a stub (TODO comment)
- **Fix**: Call the cardano.go functions in sequence:
  1. `GetUTxOs()` to get available inputs
  2. `BuildTransaction()` to build mint TX
  3. `SignTransaction()` to sign
  4. `SubmitTransaction()` to submit
  5. `SendNFT()` to transfer NFT to sender

## Dev & Test Workflow Notes

### Local Testing with Mock Deposits
```bash
# 1. Create mock_deposits.json
echo '[{"monitor":"addr1vx...","sender":"addr1xy...","amount":27000000,"tx":"test_tx_1"}]' > mock_deposits.json

# 2. Set required env vars
export MONITOR_ADDRESS="addr1vx..."
export POLICY_ID="abc123..."
export SCRIPT_FILE="./policy.script"
export METADATA_FILE="./metadata.json"

# 3. Run engine
go run main.go
```

### State File Inspection
```bash
cat flowmass.state  # JSON: {"next_mint_counter": 5, "processed_deposits": [...]}
```

### Blockfrost Integration (Production)
```bash
export BLOCKFROST_API_KEY="<your_key>"
export BLOCKFROST_NETWORK="mainnet"  # or testnet
go run main.go
```

## Safety & Production Checklist

- [ ] Implement full `cardano-cli` transaction building / signing / submission
- [ ] Resolve sender address from Blockfrost UTxO inputs
- [ ] Test minting on testnet with real cardano-cli
- [ ] Secure private key storage (NOT in code; use environment or secure vault)
- [ ] Add retry logic for failed deposits
- [ ] Database persistence for NFTs and state (not just JSON file)
- [ ] Add observability: logging, metrics, alerting
- [ ] Validate metadata.json and script.script exist and are valid
- [ ] Handle invalid-hereafter slot calculations correctly

## If Anything Is Unclear

Ask me to:
- Implement a specific `cardano-cli` wrapper in `cardano.go`
- Resolve sender addresses from Blockfrost
- Handle different Cardano eras or networks
- Add database persistence
- Set up proper error recovery and retries

The stubs in `cardano.go` are ready for you to fill in.

