package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GetCurrentSlot queries the current Cardano slot number.
func GetCurrentSlot() (int64, error) {
	args := []string{"query", "tip", "--mainnet"}

	cmd := exec.Command("cardano-cli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to query tip: %w", err)
	}

	// Parse JSON response
	var result struct {
		Slot int64 `json:"slot"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("failed to parse slot from response: %w", err)
	}

	return result.Slot, nil
}

// BuildTransaction constructs a Cardano transaction with minting.
func BuildTransaction(utxoIn, monitorAddr, recipientAddr, nftName, policyID, scriptFile, metadataFile string, invalidHereafter int64) (string, error) {
	txFile := "/tmp/tx.raw"

	// Prepare mint specification
	mintSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)

	args := []string{
		"transaction", "build",
		"--babbage-era",
		"--mainnet",
		"--tx-in", utxoIn,
		"--mint", mintSpec,
		"--minting-script-file", scriptFile,
		"--tx-out", fmt.Sprintf("%s+0", recipientAddr),
		"--invalid-hereafter", strconv.FormatInt(invalidHereafter, 10),
		"--metadata-json-file", metadataFile,
		"--out-file", txFile,
	}

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to build transaction: %w (output: %s)", err, string(output))
	}

	return txFile, nil
}

// SignTransaction signs a transaction.
func SignTransaction(txFile, signingKeyFile string) (string, error) {
	signedFile := strings.TrimSuffix(txFile, filepath.Ext(txFile)) + ".signed"

	args := []string{
		"transaction", "sign",
		"--tx-body-file", txFile,
		"--signing-key-file", signingKeyFile,
		"--mainnet",
		"--out-file", signedFile,
	}

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w (output: %s)", err, string(output))
	}

	return signedFile, nil
}

// SubmitTransaction submits a signed transaction to the blockchain.
func SubmitTransaction(signedFile string) (string, error) {
	args := []string{
		"transaction", "submit",
		"--mainnet",
		"--tx-file", signedFile,
	}

	cmd := exec.Command("cardano-cli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w (output: %s)", err, string(out))
	}

	// Extract transaction hash from output
	// Typical output: "Transaction successfully submitted."
	output := strings.TrimSpace(string(out))
	return output, nil
}

// GetUTxOs queries available UTxOs at an address.
func GetUTxOs(address string) ([]string, error) {
	utxoFile := "/tmp/utxos.json"

	args := []string{
		"query", "utxo",
		"--address", address,
		"--mainnet",
		"--out-file", utxoFile,
	}

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to query utxos: %w (output: %s)", err, string(output))
	}

	// Parse UTxOs from JSON file
	data, err := ioutil.ReadFile(utxoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read utxos file: %w", err)
	}

	var utxoMap map[string]interface{}
	if err := json.Unmarshal(data, &utxoMap); err != nil {
		return nil, fmt.Errorf("failed to parse utxos json: %w", err)
	}

	// Extract UTXO keys (format: "txhash#index")
	var utxos []string
	for key := range utxoMap {
		utxos = append(utxos, key)
	}

	if len(utxos) == 0 {
		return nil, fmt.Errorf("no UTxOs found at address %s", address)
	}

	return utxos, nil
}

// SendNFT constructs and submits a transaction to send an NFT to recipient.
func SendNFT(nftID, recipientAddr, signingKeyFile string) (string, error) {
	// TODO: Implement full NFT transfer workflow
	// 1. Get sender UTxO containing the NFT
	// 2. Build transaction with NFT output to recipient
	// 3. Sign and submit
	return "", fmt.Errorf("not yet implemented")
}
