package server

import (
	"errors"
	"fmt"
)

func fetchLatestBlocks(n int) ([]map[string]interface{}, error) {
	// Get the latest block height
	networkStatus, err := getNetworkStatus()
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	result, ok := networkStatus["result"].(map[string]interface{})
	if !ok {
		return nil, errors.New("could not parse result from network status")
	}
	latestHeight, ok := result["best_block_height"].(float64)
	if !ok {
		return nil, errors.New("could not parse latest block height")
	}

	blocks := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		height := int(latestHeight) - i
		block, err := getBlockDetails(fmt.Sprintf("%d", height))
		if err != nil {
			continue // skip errors, e.g., for orphaned blocks
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// Fetch the latest N transactions (from the latest N blocks)
func fetchLatestTransactions(nBlocks, nTxs int) ([]map[string]interface{}, error) {
	blocks, err := fetchLatestBlocks(nBlocks)
	if err != nil {
		return nil, err
	}
	transactions := make([]map[string]interface{}, 0, nTxs)
	for _, block := range blocks {
		// Access the nested structure properly
		blockData, ok := block["result"].(map[string]interface{})
		if !ok {
			continue
		}

		txs, ok := blockData["tx"]
		if !ok {
			continue
		}
		txList, ok := txs.([]interface{})
		if !ok {
			continue
		}
		for _, txid := range txList {
			if len(transactions) >= nTxs {
				return transactions, nil
			}
			txDetail, err := getTransactionDetails(fmt.Sprintf("%v", txid))
			if err != nil {
				continue
			}
			transactions = append(transactions, txDetail)
		}
	}
	return transactions, nil
}
