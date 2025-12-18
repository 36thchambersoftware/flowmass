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
	// legacy default to mainnet; prefer calling code to pass network-aware variant
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

// netArgs returns CLI flags for the selected network.
func netArgs(network, testnetMagic string) []string {
	if network == "mainnet" || network == "" {
		return []string{"--mainnet"}
	}
	// preprod / test networks require testnet-magic
	return []string{"--testnet-magic", testnetMagic}
}

// GetCurrentSlotNetwork queries the current slot for the specified network.
func GetCurrentSlotNetwork(network, testnetMagic string) (int64, error) {
	args := []string{"query", "tip"}
	args = append(args, netArgs(network, testnetMagic)...)

	cmd := exec.Command("cardano-cli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to query tip: %w", err)
	}

	var result struct {
		Slot int64 `json:"slot"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("failed to parse slot from response: %w", err)
	}

	return result.Slot, nil
}

// BuildTransaction constructs a Cardano transaction with minting.
func BuildTransaction(utxoIns []string, monitorAddr, recipientAddr, nftName, policyID, scriptFile, metadataFile string, invalidHereafter int64, network, testnetMagic string) (string, error) {
	txFile := "/tmp/tx.raw"

	// Prepare mint specification
	mintSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)

	args := []string{
		"conway", "transaction", "build",
	}

	// add all inputs
	for _, in := range utxoIns {
		args = append(args, "--tx-in", in)
	}

	// --tx-out addr1qyfy6z5q2c370kju53dtjw6qwwmlt7tdjscjj97zval0668ueyljyfjl4lh2pdynrfz4a6mu4xdjyetzmyezugud4epqak50kt+1400000+"1 1d0cf168b30d27c6619e7ca7c18e02c8cebc011bf056216a1ea829ff.466c6f776d6173732039"
	// Build tx-out with min-ADA and the minted asset.
	// Use a conservative min-ADA value for NFT outputs (1_400_000 lovelace)
	minUtxo := int64(1400000)
	// Format: addr+minUtxo+"1 policyId.tokenName"
	assetSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)
	txOut := fmt.Sprintf("%s+%d+\"%s\"", recipientAddr, minUtxo, assetSpec)

	args = append(args,
		"--mint", mintSpec,
		"--minting-script-file", scriptFile,
		"--tx-out", txOut,
		"--invalid-hereafter", strconv.FormatInt(invalidHereafter, 10),
		"--metadata-json-file", metadataFile,
		"--change-address", monitorAddr,
		"--witness-override", "1",
		"--out-file", txFile,
	)

	// append network args
	args = append(args, netArgs(network, testnetMagic)...)

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to build transaction: %w (output: %s)", err, string(output))
	}

	return txFile, nil
}

// SignTransaction signs a transaction.
func SignTransaction(txFile, signingKeyFile, network, testnetMagic string) (string, error) {
	signedFile := strings.TrimSuffix(txFile, filepath.Ext(txFile)) + ".signed"

	args := []string{
		"transaction", "sign",
		"--tx-body-file", txFile,
		"--signing-key-file", signingKeyFile,
		"--out-file", signedFile,
	}

	// append network args
	args = append(args, netArgs(network, testnetMagic)...)

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w (output: %s)", err, string(output))
	}

	return signedFile, nil
}

// SubmitTransaction submits a signed transaction to the blockchain.
func SubmitTransaction(signedFile, network, testnetMagic string) (string, error) {
	args := []string{
		"transaction", "submit",
		"--tx-file", signedFile,
	}

	args = append(args, netArgs(network, testnetMagic)...)

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

// UTxO represents a parsed UTxO with lovelace and any other assets.
type UTxO struct {
	ID       string
	Lovelace int64
	Assets   map[string]uint64 // non-lovelace assets (policyid.assetname -> quantity)
}

// GetUTxOs queries available UTxOs at an address.
func GetUTxOs(address, network, testnetMagic string) ([]UTxO, error) {
	utxoFile := "/tmp/utxos.json"

	args := []string{
		"query", "utxo",
		"--address", address,
		"--out-file", utxoFile,
	}

	args = append(args, netArgs(network, testnetMagic)...)

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to query utxos: %w (output: %s)", err, string(output))
	}

	data, err := ioutil.ReadFile(utxoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read utxos file: %w", err)
	}

	// Try the common JSON shape: map[string][]{{unit,quantity}}
	var raw map[string][]struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	}

	var result []UTxO
	if err := json.Unmarshal(data, &raw); err == nil {
		for k, amounts := range raw {
			var lov int64
			assets := make(map[string]uint64)
			for _, a := range amounts {
				if a.Unit == "lovelace" {
					fmt.Sscanf(a.Quantity, "%d", &lov)
				} else {
					if q, err := strconv.ParseUint(a.Quantity, 10, 64); err == nil {
						assets[a.Unit] = q
					} else {
						assets[a.Unit] = 0
					}
				}
			}
			result = append(result, UTxO{ID: k, Lovelace: lov, Assets: assets})
		}
	} else {
		// Fallback: unmarshal into generic map and mark as unknown assets
		var generic map[string]interface{}
		if err := json.Unmarshal(data, &generic); err != nil {
			return nil, fmt.Errorf("failed to parse utxos json: %w", err)
		}
		for k := range generic {
			// unknown amounts; make Assets non-nil so callers treat as unusable
			result = append(result, UTxO{ID: k, Lovelace: 0, Assets: map[string]uint64{"unknown": 1}})
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no UTxOs found at address %s", address)
	}

	return result, nil
}

// SendNFT constructs and submits a transaction to send an NFT to recipient.
func SendNFT(nftID, recipientAddr, signingKeyFile string) (string, error) {
	// TODO: Implement full NFT transfer workflow
	// 1. Get sender UTxO containing the NFT
	// 2. Build transaction with NFT output to recipient
	// 3. Sign and submit
	return "", fmt.Errorf("not yet implemented")
}
