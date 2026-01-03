package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Engine orchestrates deposit monitoring and NFT minting.
type Engine struct {
	monitorAddr string
	mintPrice   int64
	policyID    string
	scriptFile  string
	// metadataFile   string
	state          *State
	blockfrostKey  string
	network        string
	testnetMagic   string
	signingKeyFile string
	quit           chan struct{}
}

// NewEngine creates a new minting engine.
func NewEngine(monitorAddr string, mintPrice int64, policyID, scriptFile, stateFile, blockfrostKey, network, testnetMagic, signingKeyFile string) (*Engine, error) {
	// Load or initialize state
	state, err := LoadState(stateFile)
	if err != nil {
		return nil, err
	}

	// Ensure cardano-cli is present and can query the local node tip.
	if err := ensureCardanoCLIAvailable(network, testnetMagic); err != nil {
		return nil, err
	}

	// If we have a Blockfrost key, sync next mint counter with on-chain assets
	if blockfrostKey == "" {
		return nil, fmt.Errorf("no blockfrost key provided; skipping on-chain sync")
	}
	maxOnChain, err := getMaxOnChainFlowmass(policyID, blockfrostKey, network)
	if err == nil && maxOnChain+1 > state.NextMintCounter {
		state.mu.Lock()
		state.NextMintCounter = maxOnChain + 1
		state.mu.Unlock()
		if err := state.Save(); err != nil {
			return nil, fmt.Errorf("failed to save state after syncing on-chain")
		} else {
			log.Printf("[engine] synced next_mint_counter to %d based on on-chain assets", state.NextMintCounter)
		}
	}

	// Reconcile any pending reservations saved from a previous run.
	if len(state.PendingDeposits) > 0 {
		if maxOnChain == 0 {
			// try to fetch maxOnChain if not already available
			if m, merr := getMaxOnChainFlowmass(policyID, blockfrostKey, network); merr == nil {
				maxOnChain = m
			}
		}
		for tx, id := range state.PendingDeposits {
			if maxOnChain >= id {
				log.Printf("[engine] pending reservation for tx %s (id=%d) appears minted on-chain; marking processed", tx, id)
				state.MarkProcessed(tx)
				if err := state.ClearPending(tx); err != nil {
					log.Printf("[engine] warning: failed to clear pending for %s: %v", tx, err)
				}
				if err := state.Save(); err != nil {
					log.Printf("[engine] warning: failed to save state while reconciling pending: %v", err)
				}
			}
		}
	}

	return &Engine{
		monitorAddr: monitorAddr,
		mintPrice:   mintPrice,
		policyID:    policyID,
		scriptFile:  scriptFile,
		// metadataFile:   metadataFile,
		state:          state,
		blockfrostKey:  blockfrostKey,
		network:        network,
		testnetMagic:   testnetMagic,
		signingKeyFile: signingKeyFile,
		quit:           make(chan struct{}),
	}, nil
}

// Start begins the deposit polling loop.
func (e *Engine) Start() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	log.Println("[engine] Starting deposit polling (60s interval)")

	// Do an immediate poll on startup so we don't wait for the first tick.
	go func() {
		e.pollDeposits()
	}()

	for {
		select {
		case <-ticker.C:
			e.pollDeposits()
		case <-e.quit:
			log.Println("[engine] Stopping")
			return
		}
	}
}

// Stop signals the engine to halt.
func (e *Engine) Stop() {
	close(e.quit)
}

// pollDeposits checks for new 27 ADA deposits and mints NFTs.
func (e *Engine) pollDeposits() {
	log.Println("[engine] poll tick")
	deposits, err := e.fetchDeposits()
	if err != nil {
		log.Printf("[engine] error fetching deposits: %v", err)
		return
	}

	for _, dep := range deposits {
		// Check if already processed
		if e.state.IsProcessed(dep.TxHash) {
			continue
		}

		log.Printf("[engine] found deposit: %s -> %d lovelace (tx=%s)", dep.SenderAddr, dep.Amount, dep.TxHash)

		// Mint NFT for this deposit
		if err := e.mintNFTForDeposit(dep); err != nil {
			log.Printf("[engine] failed to mint for deposit %s: %v", dep.TxHash, err)
			continue
		}

		// Mark processed
		e.state.MarkProcessed(dep.TxHash)
		if err := e.state.Save(); err != nil {
			log.Printf("[engine] warning: failed to save state: %v", err)
		}

		log.Printf("[engine] successfully minted NFT for deposit %s", dep.TxHash)
	}
}

// fetchDeposits retrieves unprocessed deposits matching the mint price.
func (e *Engine) fetchDeposits() ([]Deposit, error) {
	return e.fetchDepositsBlockfrost()
}

// fetchDepositsBlockfrost queries Blockfrost for UTxOs.
func (e *Engine) fetchDepositsBlockfrost() ([]Deposit, error) {
	lovelaceTarget := e.mintPrice
	var base string
	if e.network == "mainnet" {
		base = "https://cardano-mainnet.blockfrost.io/api/v0"
	} else {
		base = "https://cardano-preprod.blockfrost.io/api/v0"
	}
	url := fmt.Sprintf("%s/addresses/%s/utxos", base, e.monitorAddr)
	log.Printf("[engine] fetching deposits from Blockfrost URL=%s", url)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "curl", "-s",
		"-H", fmt.Sprintf("project_id:%s", e.blockfrostKey),
		url)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("blockfrost curl failed: %v; output: %s", err, strings.TrimSpace(string(out)))
	}

	// Try parsing expected array response first
	var utxos []struct {
		TxHash string `json:"tx_hash"`
		Amount []struct {
			Unit     string `json:"unit"`
			Quantity string `json:"quantity"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(out, &utxos); err != nil {
		// Not the expected array — likely an error object from Blockfrost.
		// Try to parse a common error shape and return a helpful message.
		var errObj map[string]interface{}
		if jerr := json.Unmarshal(out, &errObj); jerr == nil {
			// include the full body in the error for easier debugging
			return nil, fmt.Errorf("unexpected Blockfrost response for %s: %v", url, errObj)
		}
		// Last resort: return raw body as string
		return nil, fmt.Errorf("failed to parse Blockfrost utxos response: %v; raw=%s", err, strings.TrimSpace(string(out)))
	}

	var deposits []Deposit
	for _, u := range utxos {
		if e.state.IsProcessed(u.TxHash) {
			continue
		}
		// Parse lovelace amount
		var lovelace int64
		for _, a := range u.Amount {
			if a.Unit == "lovelace" {
				fmt.Sscanf(a.Quantity, "%d", &lovelace)
			}
		}
		if lovelace == lovelaceTarget {
			// Resolve sender from transaction inputs via Blockfrost /txs/{hash}/utxos
			sender := "unknown"
			txCtx, txCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer txCancel()
			txCmd := exec.CommandContext(txCtx, "curl", "-s",
				"-H", fmt.Sprintf("project_id:%s", e.blockfrostKey),
				fmt.Sprintf("%s/txs/%s/utxos", base, u.TxHash))
			if txOut, err := txCmd.CombinedOutput(); err == nil {
				var txDetails struct {
					Inputs []struct {
						Address string `json:"address"`
					} `json:"inputs"`
				}
				if err := json.Unmarshal(txOut, &txDetails); err == nil && len(txDetails.Inputs) > 0 {
					sender = txDetails.Inputs[0].Address
				}
			} else {
				log.Printf("[engine] warning: failed to resolve tx sender for %s: %v; out=%s", u.TxHash, err, strings.TrimSpace(string(txOut)))
			}

			deposits = append(deposits, Deposit{
				TxHash:     u.TxHash,
				SenderAddr: sender,
				Amount:     lovelace,
			})
		}
	}
	return deposits, nil
}

// getMaxOnChainFlowmass queries Blockfrost for assets under the policy and
// returns the maximum index N found for asset names decoding to "Flowmass N".
func getMaxOnChainFlowmass(policyID, blockfrostKey, network string) (int, error) {
	var base string
	if network == "mainnet" {
		base = "https://cardano-mainnet.blockfrost.io/api/v0"
	} else {
		base = "https://cardano-preprod.blockfrost.io/api/v0"
	}
	max := 0
	// fetch several pages to be safer (pagination)
	for page := 1; page <= 100; page++ {
		// fetch this page
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, "curl", "-s",
			"-H", fmt.Sprintf("project_id:%s", blockfrostKey),
			fmt.Sprintf("%s/assets/policy/%s?page=%d", base, policyID, page))
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return max, fmt.Errorf("blockfrost assets fetch failed: %v; output: %s", err, strings.TrimSpace(string(out)))
		}

		var assets []struct {
			Asset    string `json:"asset"`
			Quantity string `json:"quantity"`
		}
		if err := json.Unmarshal(out, &assets); err != nil {
			// try to report the body for easier debugging
			var errObj map[string]interface{}
			if jerr := json.Unmarshal(out, &errObj); jerr == nil {
				return max, fmt.Errorf("unexpected Blockfrost response for assets (page=%d): %v", page, errObj)
			}
			return max, fmt.Errorf("failed to parse Blockfrost assets response (page=%d): %v; raw=%s", page, err, strings.TrimSpace(string(out)))
		}
		if len(assets) == 0 {
			break
		}

		for _, a := range assets {
			// Blockfrost /assets/policy returns objects with `asset` which is
			// policyID + hex(asset_name). Extract hex suffix and decode it.
			assetStr := a.Asset
			var assetNameHex string
			if strings.HasPrefix(assetStr, policyID) {
				assetNameHex = assetStr[len(policyID):]
			} else {
				// fallback: if asset doesn't start with policyID, try to parse whole string
				assetNameHex = assetStr
			}

			if assetNameHex == "" {
				continue
			}
			if b, err := hex.DecodeString(assetNameHex); err == nil {
				name := string(b)
				if strings.HasPrefix(name, "Flowmass ") {
					var n int
					if _, err := fmt.Sscanf(name, "Flowmass %d", &n); err == nil {
						if n > max {
							max = n
						}
					}
				}
			}
		}
	}
	return max, nil
}

// ensureCardanoCLIAvailable checks that `cardano-cli` is in PATH and that
// `cardano-cli query tip` succeeds for the configured network. The engine
// requires a working cardano node and CLI in order to mint.
func ensureCardanoCLIAvailable(network, testnetMagic string) error {
	if _, err := exec.LookPath("cardano-cli"); err != nil {
		return fmt.Errorf("cardano-cli not found in PATH: %v", err)
	}

	args := []string{"query", "tip"}
	// append network and socket args (socketAndNetArgs validates testnetMagic and socket)
	netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
	if err != nil {
		return err
	}
	args = append(args, netArgsWithSocket...)
	cmd := exec.Command("cardano-cli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cardano-cli query tip failed: %v; output: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// fetchDepositsMock reads from mock_deposits.json for testing.
func (e *Engine) fetchDepositsMock() ([]Deposit, error) {
	const mockFile = "mock_deposits.json"
	data, err := os.ReadFile(mockFile)
	if err != nil {
		// No mock file; return empty
		return []Deposit{}, nil
	}

	var mockDeposits []struct {
		Monitor    string `json:"monitor"`
		SenderAddr string `json:"sender"`
		Amount     int64  `json:"amount"`
		TxHash     string `json:"tx"`
	}
	if err := json.Unmarshal(data, &mockDeposits); err != nil {
		return nil, err
	}

	var deposits []Deposit
	lovelaceTarget := e.mintPrice
	for _, m := range mockDeposits {
		if m.Monitor != e.monitorAddr || e.state.IsProcessed(m.TxHash) {
			continue
		}
		if m.Amount == lovelaceTarget {
			deposits = append(deposits, Deposit{
				TxHash:     m.TxHash,
				SenderAddr: m.SenderAddr,
				Amount:     m.Amount,
			})
		}
	}
	return deposits, nil
}

// mintNFTForDeposit orchestrates the full minting workflow.
func (e *Engine) mintNFTForDeposit(dep Deposit) error {
	log.Printf("[engine] minting NFT for sender %s (tx=%s)", dep.SenderAddr, dep.TxHash)

	// Reserve and persist the next mint id for this deposit to avoid gaps
	id, rerr := e.state.ReservePendingMint(dep.TxHash)
	if rerr != nil {
		return fmt.Errorf("failed to reserve mint id: %v", rerr)
	}
	// Display name and hex-encoded on-chain asset name
	displayName := fmt.Sprintf("Flowmass %d", id)
	hexName := hex.EncodeToString([]byte(displayName))

	// Get current slot
	slot, err := GetCurrentSlotNetwork(e.network, e.testnetMagic)
	if err != nil {
		return fmt.Errorf("failed to get current slot: %v", err)
	}
	invalidHereafter := slot + 10000

	log.Printf("[engine] minting %s (hex=%s) (slot=%d, invalid-hereafter=%d)", displayName, hexName, slot, invalidHereafter)

	// 1. Get UTxO from monitor address (choose lovelace-only UTxOs that cover mint + fee buffer)
	utxos, err := GetUTxOs(e.monitorAddr, e.network, e.testnetMagic)
	if err != nil {
		return fmt.Errorf("failed to get utxos: %v", err)
	}

	// collect strict lovelace-only candidates (no non-lovelace assets at all)
	var candidates []UTxO
	for _, u := range utxos {
		if (u.Assets == nil || len(u.Assets) == 0) && u.Lovelace > 0 {
			candidates = append(candidates, u)
		}
	}
	if len(candidates) == 0 {
		// debug: report counts and sample UTxOs to help operator diagnose
		total := len(utxos)
		withAssets := 0
		withLovelace := 0
		for _, u := range utxos {
			if u.Lovelace > 0 {
				withLovelace++
			}
			if u.Assets != nil && len(u.Assets) > 0 {
				withAssets++
			}
		}
		log.Printf("[engine] debug: total_utxos=%d lovelace_utxos=%d utxos_with_assets=%d", total, withLovelace, withAssets)
		for i, u := range utxos {
			if i >= 8 {
				break
			}
			log.Printf("[engine] debug utxo[%d]: id=%s lovelace=%d assets=%v", i, u.ID, u.Lovelace, u.Assets)
		}
		return fmt.Errorf("no lovelace-only UTxO available at monitor address")
	}

	// sort descending by lovelace to minimize inputs
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Lovelace > candidates[j].Lovelace })

	// require mint price + buffer (2 ADA) to cover fees and change
	required := uint64(e.mintPrice + 2000000)
	var selectedIns []string
	var sum uint64
	for _, c := range candidates {
		selectedIns = append(selectedIns, c.ID)
		sum += c.Lovelace
		if sum >= required {
			break
		}
	}
	if sum < required {
		return fmt.Errorf("insufficient lovelace in lovelace-only UTxOs: have=%d required=%d", sum, required)
	}

	log.Printf("[engine] selected UTxOs: %v (total lovelace=%d)", selectedIns, sum)

	// 2. Build mint transaction
	txFile, err := BuildTransaction(
		selectedIns,
		e.monitorAddr,
		dep.SenderAddr,
		hexName,
		e.policyID,
		e.scriptFile,
		// e.metadataFile,
		invalidHereafter,
		e.network,
		e.testnetMagic,
	)
	if err != nil {
		return fmt.Errorf("failed to build transaction: %v", err)
	}
	log.Printf("[engine] built transaction: %s", txFile)

	// 3. Sign transaction
	signedFile, err := SignTransaction(txFile, e.signingKeyFile, e.network, e.testnetMagic)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}
	log.Printf("[engine] signed transaction: %s", signedFile)

	// 4. Submit transaction
	txHash, err := SubmitTransaction(signedFile, e.network, e.testnetMagic)
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %v", err)
	}
	log.Printf("[engine] submitted transaction: %s", txHash)

	// Mark deposit processed and clear pending reservation (persisting both changes)
	e.state.MarkProcessed(dep.TxHash)
	if err := e.state.ClearPending(dep.TxHash); err != nil {
		// ClearPending persists state; if it fails, attempt a Save and warn
		log.Printf("[engine] warning: failed to clear pending reservation: %v", err)
		if serr := e.state.Save(); serr != nil {
			log.Printf("[engine] warning: failed to save state after marking processed: %v", serr)
		}
	}

	Webhook(fmt.Sprintf("Minted NFT: %s", displayName))

	return nil
}

// Deposit represents an incoming ADA transfer.
type Deposit struct {
	TxHash     string
	SenderAddr string
	Amount     int64
}
