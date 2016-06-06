// Defines blockDataCollector and stakeInfoDataCollector, the client
// controllers; blockData and stakeInfoData, the data structures returned by
// the collect() methods.
//
// To-do: mempool collector for ticket count/fee info.
//
// chappjc

package main

import (
	"encoding/hex"
	"fmt"
	//"sync/atomic"
	"strconv"

	"github.com/decred/dcrd/dcrjson"
	//"github.com/decred/dcrd/wire"
	//"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

// stakeInfoData
type stakeInfoData struct {
	height    uint32
	stakeinfo *dcrjson.GetStakeInfoResult
}

type stakeInfoDataCollector struct {
	cfg              *config
	dcrdChainSvr     *dcrrpcclient.Client
	dcrwChainSvr     *dcrrpcclient.Client
	priceWindowNum   int // TODO: Use this information.
	idxBlockInWindow int // Relative block index within the difficulty period
}

// newStakeInfoDataCollector creates a new stakeInfoDataCollector.
func newStakeInfoDataCollector(cfg *config,
	dcrdChainSvr *dcrrpcclient.Client,
	dcrwChainSvr *dcrrpcclient.Client) (*stakeInfoDataCollector, error) {
	return &stakeInfoDataCollector{
		cfg:          cfg,
		dcrdChainSvr: dcrdChainSvr,
		dcrwChainSvr: dcrwChainSvr,
	}, nil
}

// collect is the main handler for collecting chain data
func (t *stakeInfoDataCollector) collect() (*stakeInfoData, error) {
	winSize := uint32(activeNet.StakeDiffWindowSize)

	// Make sure that our wallet is connected to the daemon.
	if t.dcrwChainSvr != nil {
		walletInfo, err := t.dcrwChainSvr.WalletInfo()
		if err != nil {
			return nil, err
		}

		if !walletInfo.DaemonConnected {
			return nil, fmt.Errorf("Wallet not connected to daemon")
		}
	}

	// block height
	blockCount, err := t.dcrdChainSvr.GetBlockCount()
	if err != nil {
		return nil, err
	}
	height := uint32(blockCount)

	t.idxBlockInWindow = int(height % winSize)
	t.priceWindowNum = int(height / winSize)

	// Stake Info
	getStakeInfoRes, err := t.dcrwChainSvr.GetStakeInfo()
	if err != nil {
		return nil, err
	}

	// Output
	stakeinfo := &stakeInfoData{
		height:    height,
		stakeinfo: getStakeInfoRes,
	}

	return stakeinfo, err
}

// TicketPoolInfo models data about ticket pool
type TicketPoolInfo struct {
	PoolSize   uint32  `json:"poolsize"`
	PoolValue  float64 `json:"poolvalue"`
	PoolValAvg float64 `json:"poolvalavg"`
}

// blockData
// consider if pointers are desirable here
type blockData struct {
	header           dcrjson.GetBlockHeaderVerboseResult
	feeinfo          dcrjson.FeeInfoBlock
	currentstakediff dcrjson.GetStakeDifficultyResult
	eststakediff     dcrjson.EstimateStakeDiffResult
	poolinfo         TicketPoolInfo
}

type blockDataCollector struct {
	cfg              *config
	dcrdChainSvr     *dcrrpcclient.Client
	priceWindowNum   int
	idxBlockInWindow int // Relative block index within the difficulty period
}

// newBlockDataCollector creates a new blockDataCollector.
func newBlockDataCollector(cfg *config,
	dcrdChainSvr *dcrrpcclient.Client) (*blockDataCollector, error) {
	return &blockDataCollector{
		cfg:          cfg,
		dcrdChainSvr: dcrdChainSvr,
	}, nil
}

// collect is the main handler for collecting chain data
func (t *blockDataCollector) collect() (*blockData, error) {
	winSize := uint32(activeNet.StakeDiffWindowSize)

	// Pull and store relevant data about the blockchain.
	bestBlockHash, err := t.dcrdChainSvr.GetBestBlockHash()
	if err != nil {
		return nil, err
	}
	bestBlock, err := t.dcrdChainSvr.GetBlock(bestBlockHash)
	if err != nil {
		return nil, err
	}

	blockHeader := bestBlock.MsgBlock().Header

	height := blockHeader.Height
	t.idxBlockInWindow = int(height % winSize)
	t.priceWindowNum = int(height / winSize)

	poolSize := blockHeader.PoolSize
	//timestamp := blockHeader.Timestamp

	poolValue, err := t.dcrdChainSvr.GetTicketPoolValue()
	if err != nil {
		return nil, err
	}
	avgPricePoolAmt := poolValue / dcrutil.Amount(poolSize)

	tiketPoolInfo := TicketPoolInfo{poolSize, poolValue.ToCoin(),
		avgPricePoolAmt.ToCoin()}

	// Fee info
	numFeeBlocks := uint32(1)
	numFeeWindows := uint32(0)

	feeInfo, err := t.dcrdChainSvr.TicketFeeInfo(&numFeeBlocks, &numFeeWindows)
	if err != nil {
		return nil, err
	}

	if len(feeInfo.FeeInfoBlocks) == 0 {
		return nil, fmt.Errorf("Unable to get fee info for block %d", height)
	}
	feeInfoBlock := feeInfo.FeeInfoBlocks[0]

	// Stake difficulty
	stakeDiff, err := t.dcrdChainSvr.GetStakeDifficulty()
	if err != nil {
		return nil, err
	}

	// To get difficulty, use getinfo or getmininginfo
	info, err := t.dcrdChainSvr.GetInfo()

	// blockVerbose, err := t.dcrdChainSvr.GetBlockVerbose(bestBlockHash, false)
	// if err != nil {
	// 	log.Error(err)
	// }

	// We want a GetBlockHeaderVerboseResult
	// Not sure how to manage this:
	//cmd := dcrjson.NewGetBlockHeaderCmd(bestBlockHash.String(), dcrjson.Bool(true))
	// instead:
	blockHeaderResults := dcrjson.GetBlockHeaderVerboseResult{
		Hash:          bestBlockHash.String(),
		Confirmations: uint64(1),
		Version:       blockHeader.Version,
		PreviousHash:  blockHeader.PrevBlock.String(),
		MerkleRoot:    blockHeader.MerkleRoot.String(),
		StakeRoot:     blockHeader.StakeRoot.String(),
		VoteBits:      blockHeader.VoteBits,
		FinalState:    hex.EncodeToString(blockHeader.FinalState[:]),
		Voters:        blockHeader.Voters,
		FreshStake:    blockHeader.FreshStake,
		Revocations:   blockHeader.Revocations,
		PoolSize:      blockHeader.PoolSize,
		Bits:          strconv.FormatInt(int64(blockHeader.Bits), 16),
		SBits:         dcrutil.Amount(blockHeader.SBits).ToCoin(),
		Height:        blockHeader.Height,
		Size:          blockHeader.Size,
		Time:          blockHeader.Timestamp.Unix(),
		Nonce:         blockHeader.Nonce,
		Difficulty:    info.Difficulty,
		NextHash:      "",
	}

	// estimatestakediff
	estStakeDiff, err := t.dcrdChainSvr.EstimateStakeDiff(nil)
	if err != nil {
		return nil, err
	}

	// Output
	blockdata := &blockData{
		header:           blockHeaderResults,
		feeinfo:          feeInfoBlock,
		currentstakediff: *stakeDiff,
		eststakediff:     *estStakeDiff,
		poolinfo:         tiketPoolInfo,
	}

	return blockdata, err
}
