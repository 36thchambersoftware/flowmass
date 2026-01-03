package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	blockfrostKey := flag.String("blockfrost-key", os.Getenv("BLOCKFROST_API_KEY"), "Blockfrost API key for deposit tracking")
	monitorAddr := flag.String("monitor-address", os.Getenv("MONITOR_ADDRESS"), "Cardano address to monitor for deposits")
	policyID := flag.String("policy-id", os.Getenv("POLICY_ID"), "NFT minting policy ID")
	scriptFile := flag.String("script", os.Getenv("SCRIPT_FILE"), "Path to minting script file (e.g., policy.script)")
	// metadataFile := flag.String("metadata", os.Getenv("METADATA_FILE"), "Path to metadata template JSON")
	stateFile := flag.String("state", os.Getenv("STATE_FILE"), "Path to state file (tracks mint counter and processed deposits)")
	mintPrice := flag.Int64("mint-price", 27000000, "Mint price in lovelace (default: 27000000)")
	signingKeyFile := flag.String("signing-key", os.Getenv("SIGNING_KEY_FILE"), "Path to signing key for transaction signing")
	network := flag.String("network", os.Getenv("CARDANO_NETWORK"), "Cardano network: mainnet or preprod")
	testnetMagic := flag.String("testnet-magic", os.Getenv("TESTNET_MAGIC"), "Testnet magic number for preprod (if needed)")
	flag.Parse()

	// Validate required configuration
	if *monitorAddr == "" {
		log.Fatal("monitor-address is required (use -monitor-address flag or MONITOR_ADDRESS env var)")
	}
	if *policyID == "" {
		log.Fatal("policy-id is required (use -policy-id flag or POLICY_ID env var)")
	}
	if *scriptFile == "" {
		log.Fatal("script is required (use -script flag or SCRIPT_FILE env var)")
	}
	// if *metadataFile == "" {
	// 	log.Fatal("metadata is required (use -metadata flag or METADATA_FILE env var)")
	// }
	if *stateFile == "" {
		*stateFile = "flowmass.state"
	}

	log.Println("Flowmass NFT Minting Engine (Mainnet)")
	log.Printf("Monitor Address: %s", *monitorAddr)
	log.Printf("Mint Price: %d lovelace", *mintPrice)
	log.Printf("Policy ID: %s", *policyID)
	log.Printf("Script: %s", *scriptFile)
	// log.Printf("Metadata: %s", *metadataFile)
	log.Printf("State: %s", *stateFile)
	log.Printf("Network: %s", *network)
	log.Printf("Testnet Magic: %s", *testnetMagic)

	// Initialize engine
	if *network == "" {
		*network = "mainnet"
	}
	if *network == "preprod" && *testnetMagic == "" {
		*testnetMagic = os.Getenv("TESTNET_MAGIC")
		if *testnetMagic == "" {
			*testnetMagic = "1"
		}
	}

	eng, err := NewEngine(
		*monitorAddr,
		*mintPrice,
		*policyID,
		*scriptFile,
		// *metadataFile,
		*stateFile,
		*blockfrostKey,
		*network,
		*testnetMagic,
		*signingKeyFile,
	)
	if err != nil {
		log.Fatalf("Failed to initialize engine: %v", err)
	}

	initWebhook()

	// Start engine
	go eng.Start()
	log.Println("Engine started. Press CTRL-C to exit.")

	// Wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down engine...")
	eng.Stop()
}
