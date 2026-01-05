package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
)

// Metadata represents the NFT metadata structure.
// It should match this json structure:
/*
{
	"721": {
		"1d0cf168b30d27c6619e7ca7c18e02c8cebc011bf056216a1ea829ff": {
			"466c6f776d6173732031": {
				"name": "Flowmass 1",
				"image": ["ipfs://bafybeic24satynujphugtqvwea3222g363", "ipdavlv5vhncvn6zxffrxe3e"],
				"mediaType": "image/png",
				"files": [
					{
					"name": "Flowmass",
					"mediaType": "image/png",
					"src": ["ipfs://bafybeic24satynujphugtqvwea3222g363","ipdavlv5vhncvn6zxffrxe3e"]
					}
				],
				"project": "Flowmass",
				"artist": "https://x.com/Novachrome_x377",
				"twitter": "https://x.com/PREEB_Pool",
				"discord": "https://discord.gg/aHrZJuEKZG",
				"type": "Shark"
			}
		}
	}
}
*/

// Unmarshal metadata from json string template
func MetadataTemplate(hexName string) (string, error) {
	name, err := hex.DecodeString(hexName)
	if err != nil {
		return "", err
	}

	template := fmt.Sprintf(`{
	"721": {
		"1d0cf168b30d27c6619e7ca7c18e02c8cebc011bf056216a1ea829ff": {
			"%s": {
				"name": "%s",
				"image": ["ipfs://bafybeic24satynujphugtqvwea3222g363", "ipdavlv5vhncvn6zxffrxe3e"],
				"mediaType": "image/png",
				"files": [
					{
					"name": "Flowmass",
					"mediaType": "image/png",
					"src": ["ipfs://bafybeic24satynujphugtqvwea3222g363","ipdavlv5vhncvn6zxffrxe3e"]
					}
				],
				"project": "Flowmass",
				"artist": "https://x.com/Novachrome_x377",
				"twitter": "https://x.com/PREEB_Pool",
				"discord": "https://discord.gg/aHrZJuEKZG",
				"type": "Shark"
			}
		}
	}
}`, name, name)

	return template, nil
}

// MetadatasTemplate generates metadata for multiple NFTs given a slice of hex names
func MetadatasTemplate(hexNames []string) (string, error) {
	entries := ""
	for _, hexName := range hexNames {
		name, err := hex.DecodeString(hexName)
		if err != nil {
			return "", err
		}

		entry := fmt.Sprintf(`"%s": {
				"name": "%s",
				"image": ["ipfs://bafybeic24satynujphugtqvwea3222g363", "ipdavlv5vhncvn6zxffrxe3e"],
				"mediaType": "image/png",
				"files": [
					{
					"name": "Flowmass",
					"mediaType": "image/png",
					"src": ["ipfs://bafybeic24satynujphugtqvwea3222g363","ipdavlv5vhncvn6zxffrxe3e"]
					}
				],
				"project": "Flowmass",
				"artist": "https://x.com/Novachrome_x377",
				"twitter": "https://x.com/PREEB_Pool",
				"discord": "https://discord.gg/aHrZJuEKZG",
				"type": "Shark"
			}`, name, name)

		if entries != "" {
			entries += ",\n"
		}
		entries += entry
	}

	template := fmt.Sprintf(`{
	"721": {
		"1d0cf168b30d27c6619e7ca7c18e02c8cebc011bf056216a1ea829ff": {
			%s
		}
	}
}`, entries)

	return template, nil
}

// Copy the state.go Save method to save metadata to a file to be used by cardano-cli
func SaveMetadataToFile(metadata, filePath string) error {
	return ioutil.WriteFile(filePath, []byte(metadata), 0o644)
}
