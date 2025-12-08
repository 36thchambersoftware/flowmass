package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// Engine orchestrates deposit monitoring and NFT minting.
type Engine struct {
	monitorAddr    string
	mintPrice      int64
	policyID       string
	scriptFile     string
	metadataFile   string
	state          *State
	blockfrostKey  string
	signingKeyFile string
	quit           chan struct{}
}

// NewEngine creates a new minting engine.
func NewEngine(monitorAddr string, mintPrice int64, policyID, scriptFile, metadataFile, stateFile, blockfrostKey, signingKeyFile string) (*Engine, error) {
	// Load or initialize state
	state, err := LoadState(stateFile)
	if err != nil {
		return nil, err
	}

	return &Engine{
		monitorAddr:    monitorAddr,
		mintPrice:      mintPrice,
		policyID:       policyID,
		scriptFile:     scriptFile,
		metadataFile:   metadataFile,
		state:          state,
		blockfrostKey:  blockfrostKey,
		signingKeyFile: signingKeyFile,
		quit:           make(chan struct{}),
	}, nil
}

// Start begins the deposit polling loop.
func (e *Engine) Start() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Println("[engine] Starting deposit polling (30s interval)")

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
	if e.blockfrostKey != "" {
		return e.fetchDepositsBlockfrost()
	}
	// Fallback: check local mock file for testing
	return e.fetchDepositsMock()
}

// fetchDepositsBlockfrost queries Blockfrost for UTxOs.
func (e *Engine) fetchDepositsBlockfrost() ([]Deposit, error) {
	lovelaceTarget := e.mintPrice
	base := "https://cardano-mainnet.blockfrost.io/api/v0"

	cmd := exec.Command("curl", "-s",
		"-H", fmt.Sprintf("project_id:%s", e.blockfrostKey),
		fmt.Sprintf("%s/addresses/%s/utxos", base, e.monitorAddr))

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var utxos []struct {
		TxHash string `json:"tx_hash"`
		Amount []struct {
			Unit     string `json:"unit"`
			Quantity string `json:"quantity"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(out, &utxos); err != nil {
		return nil, err
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
			// For Blockfrost, we don't have sender readily; we'd need to query tx details
			// For now, use a placeholder; in production, resolve sender from UTxO inputs
			deposits = append(deposits, Deposit{
				TxHash:     u.TxHash,
				SenderAddr: "unknown", // TODO: resolve from transaction
				Amount:     lovelace,
			})
		}
	}
	return deposits, nil
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

	// Increment mint counter
	nextID := e.state.NextMintID()
	nftName := fmt.Sprintf("nft%d", nextID)

	// Get current slot
	slot, err := GetCurrentSlot()
	if err != nil {
		return fmt.Errorf("failed to get current slot: %v", err)
	}
	invalidHereafter := slot + 10000

	log.Printf("[engine] minting %s (slot=%d, invalid-hereafter=%d)", nftName, slot, invalidHereafter)

	// 1. Get UTxO from monitor address
	utxos, err := GetUTxOs(e.monitorAddr)
	if err != nil {
		return fmt.Errorf("failed to get utxos: %v", err)
	}
	if len(utxos) == 0 {
		return fmt.Errorf("no utxos available at monitor address")
	}
	utxoIn := utxos[0]
	log.Printf("[engine] using UTXO: %s", utxoIn)

	// 2. Build mint transaction
	txFile, err := BuildTransaction(
		utxoIn,
		e.monitorAddr,
		dep.SenderAddr,
		nftName,
		e.policyID,
		e.scriptFile,
		e.metadataFile,
		invalidHereafter,
	)
	if err != nil {
		return fmt.Errorf("failed to build transaction: %v", err)
	}
	log.Printf("[engine] built transaction: %s", txFile)

	// 3. Sign transaction
	signedFile, err := SignTransaction(txFile, e.signingKeyFile)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}
	log.Printf("[engine] signed transaction: %s", signedFile)

	// 4. Submit transaction
	txHash, err := SubmitTransaction(signedFile)
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %v", err)
	}
	log.Printf("[engine] submitted transaction: %s", txHash)

	return nil
}

// Deposit represents an incoming ADA transfer.
type Deposit struct {
	TxHash     string
	SenderAddr string
	Amount     int64
}
