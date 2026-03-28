package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/correlation"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

func collectMetrics() {
	if rdb == nil {
		return
	}
	cid := correlation.NewID()
	jobLog := logging.WithComponent(logging.ComponentBackground).WithFields(log.Fields{
		logging.FieldCorrelationID: cid,
		logging.FieldEvent:         "metrics_collect",
	})
	jobLog.Debug("metrics collection run started")
	defer metrics.RecordMetricsJob()
	ctx := context.Background()
	now := float64(time.Now().Unix())

	// Collect Bitcoin price in multiple fiats for consistent portfolio charts over time
	if pricingClient != nil {
		rates, err := pricingClient.GetMultiCurrencyRatesIn(ctx, pricing.DefaultFiatCurrencies)
		if err != nil {
			jobLog.WithError(err).Warn("multi-currency rates fetch failed; using USD fallback if available")
			// Fallback: at least store USD
			if usd, err2 := pricingClient.GetBTCUSD(ctx); err2 == nil {
				s := strconv.FormatFloat(usd, 'f', -1, 64)
				rdb.ZAdd(ctx, "btc_price_history", redis.Z{Score: now, Member: s})
				rdb.ZAdd(ctx, btcPriceHistoryKey("usd"), redis.Z{Score: now, Member: s})
				rdb.ZRemRangeByRank(ctx, "btc_price_history", 0, -(btcPriceHistoryMaxPoints + 1))
				rdb.ZRemRangeByRank(ctx, btcPriceHistoryKey("usd"), 0, -(btcPriceHistoryMaxPoints + 1))
			}
		} else {
			btc, _ := rates["bitcoin"].(map[string]interface{})
			for _, c := range pricing.DefaultFiatCurrencies {
				if btc == nil {
					break
				}
				var price float64
				switch v := btc[c].(type) {
				case float64:
					price = v
				case int:
					price = float64(v)
				default:
					continue
				}
				key := btcPriceHistoryKey(c)
				rdb.ZAdd(ctx, key, redis.Z{Score: now, Member: strconv.FormatFloat(price, 'f', -1, 64)})
				rdb.ZRemRangeByRank(ctx, key, 0, -(btcPriceHistoryMaxPoints + 1))
			}
			// Legacy key for backward compatibility (USD)
			if btc != nil {
				if v, ok := btc["usd"].(float64); ok {
					rdb.ZAdd(ctx, "btc_price_history", redis.Z{Score: now, Member: strconv.FormatFloat(v, 'f', -1, 64)})
					rdb.ZRemRangeByRank(ctx, "btc_price_history", 0, -(btcPriceHistoryMaxPoints + 1))
				}
			}
		}
	}

	// Get mempool size
	mempoolResp, err := callBlockchain(context.Background(), "getmempoolinfo", []interface{}{})
	if err == nil {
		var mempoolData map[string]interface{}
		_ = json.Unmarshal(mempoolResp.Body(), &mempoolData)
		if result, ok := mempoolData["result"].(map[string]interface{}); ok {
			if size, ok := result["size"].(float64); ok {
				rdb.ZAdd(context.Background(), "mempool_size", redis.Z{Score: now, Member: size})
			}
		}
	}

	// Get latest blocks for block times and tx volume
	blocksResp, err := callBlockchain(context.Background(), "getblockchaininfo", []interface{}{})
	if err == nil {
		var chainData map[string]interface{}
		_ = json.Unmarshal(blocksResp.Body(), &chainData)
		if result, ok := chainData["result"].(map[string]interface{}); ok {
			if heightF, ok := result["blocks"].(float64); ok {
				height := int(heightF)
				// Get last 10 blocks
				blockTimes := []int64{}
				txCounts := []float64{}
				for i := 0; i < 10; i++ {
					h := height - i
					if h < 0 {
						break
					}
					blockResp, err := callBlockchain(context.Background(), "getblockhash", []interface{}{h})
					if err != nil {
						continue
					}
					var hashData map[string]interface{}
					_ = json.Unmarshal(blockResp.Body(), &hashData)
					if hash, ok := hashData["result"].(string); ok {
						blockDetailResp, err := callBlockchain(context.Background(), "getblock", []interface{}{hash})
						if err != nil {
							continue
						}
						var blockData map[string]interface{}
						_ = json.Unmarshal(blockDetailResp.Body(), &blockData)
						if result, ok := blockData["result"].(map[string]interface{}); ok {
							if t, ok := result["time"].(float64); ok {
								blockTimes = append(blockTimes, int64(t))
							}
							if txs, ok := result["tx"].([]interface{}); ok {
								txCounts = append(txCounts, float64(len(txs)))
							}
						}
					}
				}
				// Calculate average block time
				if len(blockTimes) > 1 {
					var totalTime int64 = 0
					for i := 1; i < len(blockTimes); i++ {
						// previous minus current
						totalTime += blockTimes[i-1] - blockTimes[i]
					}
					avgBlockTime := float64(totalTime) / float64(len(blockTimes)-1)
					rdb.ZAdd(context.Background(), "block_times", redis.Z{Score: now, Member: avgBlockTime})
				}
				// Sum tx volume
				if len(txCounts) > 0 {
					totalTx := float64(0)
					for _, c := range txCounts {
						totalTx += c
					}
					rdb.ZAdd(context.Background(), "tx_volume", redis.Z{Score: now, Member: totalTx})
				}
			}
		}
	}
}
