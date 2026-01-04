package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GetCurrentSlot queries the current Cardano slot number.
func GetCurrentSlot() (int64, error) {
	// delegate to network-aware variant which validates socket path
	return GetCurrentSlotNetwork("mainnet", "")
}

// netArgs returns CLI flags for the selected network.
func netArgs(network, testnetMagic string) []string {
	if network == "mainnet" || network == "" {
		return []string{"--mainnet"}
	}
	// preprod / test networks require testnet-magic
	return []string{"--testnet-magic", testnetMagic}
}

// socketAndNetArgs returns network args plus the required socket path flag.
// It reads `CARDANO_NODE_SOCKET_PATH` from the environment and returns an
// error if it's not set, since a working node socket is required.
func socketAndNetArgs(network, testnetMagic string) ([]string, error) {
	socket := os.Getenv("CARDANO_NODE_SOCKET_PATH")
	if socket == "" {
		return nil, fmt.Errorf("CARDANO_NODE_SOCKET_PATH is not set; cardano-cli requires a running node and socket path")
	}
	args := netArgs(network, testnetMagic)
	args = append(args, "--socket-path", socket)
	return args, nil
}

// GetCurrentSlotNetwork queries the current slot for the specified network.
func GetCurrentSlotNetwork(network, testnetMagic string) (int64, error) {
	args := []string{"query", "tip"}
	netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
	if err != nil {
		return 0, err
	}
	args = append(args, netArgsWithSocket...)

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
func BuildTransaction(utxoIns []string, monitorAddr, recipientAddr, nftName, policyID, scriptFile string, invalidHereafter int64, network, testnetMagic string) (string, error) {
	txFile := "/var/lib/flowmass/tx.raw"

	// Prepare mint specification
	mintSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)
	log.Printf("[cardano][mint-spec]: %s", mintSpec)

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
	minUtxo := uint64(1_400_000)
	// Format: addr+minUtxo+"1 policyId.tokenName"
	assetSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)
	txOut := fmt.Sprintf("%s+%d+%s", recipientAddr, minUtxo, assetSpec)
	log.Printf("[cardano][tx-out]: %s", txOut)

	metadata, err := MetadataTemplate(nftName)
	if err != nil {
		return "", fmt.Errorf("failed to build metadata template: %w", err)
	}
	metadataFile := fmt.Sprintf("/var/lib/flowmass/%s.json", nftName)
	SaveMetadataToFile(metadata, metadataFile)

	// Insert the NFT metadata under the correct policy ID and token name

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

	// append network args + socket
	netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
	if err != nil {
		return "", err
	}
	args = append(args, netArgsWithSocket...)
	log.Printf("[cardano][build][transaction] running cardano-cli with args: %v", args)

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
		"conway", "transaction", "sign",
		"--tx-body-file", txFile,
		"--signing-key-file", signingKeyFile,
		"--out-file", signedFile,
	}

	// append network args + socket
	netArgs := netArgs(network, testnetMagic)
	args = append(args, netArgs...)

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w (output: %s)", err, string(output))
	}

	return signedFile, nil
}

// SubmitTransaction submits a signed transaction to the blockchain.
func SubmitTransaction(signedFile, network, testnetMagic string) (string, error) {
	args := []string{
		"conway", "transaction", "submit",
		"--tx-file", signedFile,
	}
	netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
	if err != nil {
		return "", err
	}
	args = append(args, netArgsWithSocket...)

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
	Lovelace uint64
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
	netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
	if err != nil {
		return nil, err
	}
	args = append(args, netArgsWithSocket...)

	cmd := exec.Command("cardano-cli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to query utxos: %w (output: %s)", err, string(output))
	}

	data, err := ioutil.ReadFile(utxoFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read utxos file: %w", err)
	}

	var result []UTxO

	// Attempt old cardano-cli shape first: map[string][]{{unit,quantity}}
	var rawOld map[string][]struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	}
	if err := json.Unmarshal(data, &rawOld); err == nil {
		for k, amounts := range rawOld {
			var lov uint64
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
	}

	// If old shape produced nothing, try the newer cardano-cli JSON shape
	if len(result) == 0 {
		var generic map[string]map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&generic); err != nil {
			return nil, fmt.Errorf("failed to parse utxos json: %w", err)
		}

		for k, entry := range generic {
			// entry should contain a `value` map
			valueIface, ok := entry["value"]
			if !ok {
				// skip entries without value
				continue
			}
			valueMap, ok := valueIface.(map[string]interface{})
			if !ok {
				continue
			}

			var lov uint64
			assets := make(map[string]uint64)

			for unit, v := range valueMap {
				if unit == "lovelace" {
					// lovelace is always an integer; JSON decoder produced json.Number
					switch vv := v.(type) {
					case json.Number:
						if parsed, perr := vv.Int64(); perr == nil {
							lov = uint64(parsed)
						}
					case string:
						if parsed, perr := strconv.ParseUint(vv, 10, 64); perr == nil {
							lov = parsed
						}
					}
					continue
				}

				// unit is a policy id; v should be a map of assetname->quantity
				if inner, ok := v.(map[string]interface{}); ok {
					for assetName, qtyIface := range inner {
						var q uint64
						switch qv := qtyIface.(type) {
						case json.Number:
							if parsed, perr := qv.Int64(); perr == nil {
								q = uint64(parsed)
							}
						case string:
							if parsed, err := strconv.ParseUint(qv, 10, 64); err == nil {
								q = parsed
							}
						}
						// key as policyid.assetname (assetName may already be hex)
						key := unit + "." + assetName
						assets[key] = q
					}
				}
			}

			result = append(result, UTxO{ID: k, Lovelace: lov, Assets: assets})
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no UTxOs found at address %s", address)
	}

	return result, nil
}

// BuildTransactionMultipleMints constructs a Cardano transaction with multiple minting.
func BuildTransactionMultipleMints(utxoIns []string, monitorAddr, recipientAddr string, nftNames []string, policyID, scriptFile string, invalidHereafter int64, network, testnetMagic string, deposit Deposit) (string, error) {
	{
		txFile := "/var/lib/flowmass/tx.raw"

		args := []string{
			"conway", "transaction", "build",
		}

		// add all inputs
		for _, in := range utxoIns {
			args = append(args, "--tx-in", in)
		}

		// Prepare mint specification
		for _, nftName := range nftNames {
			mintSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)
			log.Printf("[cardano][mint-spec]: %s", mintSpec)
			args = append(args, "--mint", mintSpec)
		}

		// Build tx-out with min-ADA and the minted assets.
		minUtxo := uint64(1_400_000)
		for _, nftName := range nftNames {
			assetSpec := fmt.Sprintf("1 %s.%s", policyID, nftName)
			args = append(args, "--tx-out", assetSpec)
		}
		txOut := fmt.Sprintf("%s+%d", recipientAddr, minUtxo)
		log.Printf("[cardano][tx-out]: %s", txOut)

		// Prepare metadata file combining all NFTs
		combinedMetadata, err := MetadatasTemplate(nftNames)
		if err != nil {
			return "", fmt.Errorf("failed to build metadata template: %w", err)
		}

		metadataFile := fmt.Sprintf("/var/lib/flowmass/%s.json", deposit.TxHash)
		SaveMetadataToFile(combinedMetadata, metadataFile)

		args = append(args,
			"--minting-script-file", scriptFile,
			"--tx-out", txOut,
			"--invalid-hereafter", strconv.FormatInt(invalidHereafter, 10),
			"--metadata-json-file", metadataFile,
			"--change-address", monitorAddr,
			"--witness-override", strconv.Itoa(len(nftNames)),
			"--out-file", txFile,
		)

		// append network args + socket
		netArgsWithSocket, err := socketAndNetArgs(network, testnetMagic)
		if err != nil {
			return "", err
		}
		args = append(args, netArgsWithSocket...)
		log.Printf("[cardano][build][transaction] running cardano-cli with args: %v", args)

		cmd := exec.Command("cardano-cli", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("failed to build transaction: %w (output: %s)", err, string(output))
		}

		return txFile, nil
	}
}

// SendNFT constructs and submits a transaction to send an NFT to recipient.
func SendNFT(nftID, recipientAddr, signingKeyFile string) (string, error) {
	// TODO: Implement full NFT transfer workflow
	// 1. Get sender UTxO containing the NFT
	// 2. Build transaction with NFT output to recipient
	// 3. Sign and submit
	return "", fmt.Errorf("not yet implemented")
}
