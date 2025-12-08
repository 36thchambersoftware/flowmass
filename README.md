# flowmass - Cardano NFT Minting Engine

A lightweight Go engine that mints NFTs on Cardano when a monitored address receives a 27 ADA deposit. Uses `cardano-cli` to build, sign, and submit transactions.

## Features

- 🎯 Deposit-based minting (monitor for 27 ADA transfers)
- 🎨 NFT minting with custom metadata (via metadata.json)
- 🔐 Transaction signing with cardano-cli
- 💾 State tracking (mint counter, processed deposits)
- 🔄 Blockfrost support for deposit detection (or mock JSON for testing)

## Prerequisites

- Go 1.21+
- `cardano-cli` installed and in PATH
- Blockfrost API key (optional; for production deposit tracking)
- Or `mock_deposits.json` for local testing

## Configuration

Set the following environment variables or pass as CLI flags:

```bash
# Required
MONITOR_ADDRESS="addr1..."           # Cardano address to watch for deposits
POLICY_ID="..."                      # NFT minting policy ID
SCRIPT_FILE="/path/to/policy.script" # Minting script file
METADATA_FILE="/path/to/metadata.json" # Metadata template JSON
STATE_FILE="flowmass.state"          # (optional) State file for mint counter

# Optional: Blockfrost integration for mainnet deposit detection
BLOCKFROST_API_KEY="..."
BLOCKFROST_NETWORK="testnet"         # or "mainnet"
```

## Running the Engine

```bash
export MONITOR_ADDRESS="addr1..."
export POLICY_ID="abcd1234..."
export SCRIPT_FILE="./policy.script"
export METADATA_FILE="./metadata.json"

go run main.go

# Or with CLI flags
./flowmass \
  -monitor-address "addr1..." \
  -policy-id "abcd1234..." \
  -script "./policy.script" \
  -metadata "./metadata.json"
```

## Minting Workflow

1. **Monitor Address**: Engine polls for 27 ADA (27,000,000 lovelace) deposits.
2. **Deposit Detection**: Via Blockfrost (if configured) or `mock_deposits.json` (for testing).
3. **State Tracking**: Maintains mint counter (nft1, nft2, ...) and processed tx hashes.
4. **Transaction Building**:
   - Query current slot + 10,000 for invalid-hereafter
   - Get UTxO from monitored address
   - Build mint transaction with minting script
   - Build output transaction to send NFT to sender
5. **Signing & Submission**: Sign with private key, submit to blockchain.

## Example: mock_deposits.json

For local testing without Blockfrost:

```json
[
  {
    "monitor": "addr1vx...",
    "sender": "addr1xy...",
    "amount": 27000000,
    "tx": "abc123def456..."
  }
]
```

## Example: metadata.json

Template for NFT metadata (minted with each NFT):

```json
{
  "1": {
    "name": "nft1",
    "image": "ipfs://Qm...",
    "description": "Auto-minted NFT"
  }
}
```

## State File

The engine maintains a JSON state file (default: `flowmass.state`):

```json
{
  "next_mint_counter": 5,
  "processed_deposits": [
    "tx_abc...",
    "tx_def...",
    "tx_ghi..."
  ]
}
```

## Architecture

```
flowmass/
├── main.go          # Entry point, flag parsing
├── engine.go        # Deposit polling and minting orchestration
├── state.go         # State persistence (mint counter, processed deposits)
├── cardano.go       # cardano-cli command wrappers
└── README.md
```

## Key Implementation Gaps (TODO)

- [ ] Full `cardano-cli` integration for transaction building/signing/submission
- [ ] Resolve sender address from Blockfrost UTxO inputs
- [ ] Proper JSON parsing for `cardano-cli query tip` response
- [ ] NFT transfer workflow (send minted NFT to sender)
- [ ] Error recovery and retry logic
- [ ] Database persistence instead of JSON file

## Development Notes

### cardano-cli Integration

The `cardano.go` file contains placeholder/skeleton functions for:
- `GetCurrentSlot()` - Query tip to get current slot
- `BuildTransaction()` - Build mint transaction
- `SignTransaction()` - Sign with private key
- `SubmitTransaction()` - Submit to blockchain
- `GetUTxOs()` - Query UTxOs at address
- `SendNFT()` - Send minted NFT to recipient

These need full implementation using `cardano-cli` commands.

### Testing

Run locally with mock deposits:

```bash
# Create mock_deposits.json in repo root
echo '[{"monitor":"addr1...","sender":"addr1...","amount":27000000,"tx":"mock_1"}]' > mock_deposits.json

# Run engine
MONITOR_ADDRESS="addr1..." POLICY_ID="..." SCRIPT_FILE="./policy.script" METADATA_FILE="./metadata.json" go run main.go
```

## License

MIT

## Support

For issues or implementation details, open a GitHub issue.
