// Defines blockDataCollector and stakeInfoDataCollector, the client
// controllers; blockData and stakeInfoData, the data structures returned by
// the collect() methods.
//
// chappjc

package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	//"github.com/decred/dcrd/wire"
	//"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

// WalletBalances contains various wallet balances in coins
type WalletBalances struct {
	AllAllAcounts           float64 `json:"allallacounts"`
	AllDefaultAcount        float64 `json:"alldefaultacount"`
	SpendableAllAccounts    float64 `json:"spendableallaccounts"`
	SpendableDefaultAccount float64 `json:"spendabledefaultaccount"`
	LockedAllAccounts       float64 `json:"lockedallaccounts"`
	LockedImportedAccount   float64 `json:"lockedimportedaccount"`
	LockedDefaultAccount    float64 `json:"lockeddefaultaccount"`
}

// stakeInfoData
type stakeInfoData struct {
	height           uint32
	walletInfo       *dcrjson.WalletInfoResult
	stakeinfo        *dcrjson.GetStakeInfoResult
	balances         *WalletBalances
	accountBalances  *map[string]map[string]dcrutil.Amount
	priceWindowNum   int // trivia
	idxBlockInWindow int // Relative block index within the difficulty period
}

type stakeInfoDataCollector struct {
	cfg          *config
	dcrdChainSvr *dcrrpcclient.Client
	dcrwChainSvr *dcrrpcclient.Client
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

func (t stakeInfoDataCollector) getHeight() (uint32, error) {
	// block height
	blockCount, err := t.dcrdChainSvr.GetBlockCount()
	if err != nil {
		return 0, err
	}
	return uint32(blockCount), nil
}

// collect is the main handler for collecting chain data
func (t *stakeInfoDataCollector) collect(height uint32) (*stakeInfoData, error) {
	// Time this function
	defer func(start time.Time) {
		log.Tracef("stakeInfoDataCollector.collect() completed in %v",
			time.Since(start))
	}(time.Now())

	// Client pointer, simply named
	wallet := t.dcrwChainSvr

	// Make sure that our wallet is connected to the daemon.
	var err error
	var walletInfo *dcrjson.WalletInfoResult
	if wallet != nil {
		walletInfo, err = wallet.WalletInfo()
		if err != nil {
			return nil, err
		}

		if !walletInfo.DaemonConnected {
			return nil, fmt.Errorf("Wallet not connected to daemon")
		}
	}

	// block height
	// blockCount, err := t.dcrdChainSvr.GetBlockCount()
	// if err != nil {
	// 	return nil, err
	// }
	// height := uint32(blockCount)

	// Stake Info
	getStakeInfoRes, err := wallet.GetStakeInfo()
	if err != nil {
		return nil, err
	}

	accounts, err := wallet.ListAccounts()
	if err != nil {
		return nil, err
	}

	balTypes := []string{"all", "spendable", "locked"}
	accountBalances := make(map[string]map[string]dcrutil.Amount)
	for acct := range accounts {
		accountBalances[acct] = make(map[string]dcrutil.Amount)
		for _, balType := range balTypes {
			bal, err := wallet.GetBalanceMinConfType(acct, 0, balType)
			if err != nil {
				return nil, err
			}
			accountBalances[acct][balType] = bal
		}
	}

	balAllAll, err := wallet.GetBalanceMinConfType("*", 0, "all")
	balAllDefault := accountBalances["default"]["all"] // wallet.GetBalanceMinConfType("default", 0, "all")

	balSpendableAll, err := wallet.GetBalance("*")
	balSpendableDefault := accountBalances["default"]["spendable"] // wallet.GetBalance("default")

	balLockedAll, err := wallet.GetBalanceMinConfType("*", 0, "locked")
	balLockedDefault := accountBalances["default"]["locked"]   // wallet.GetBalanceMinConfType("default", 0, "locked")
	balLockedImported := accountBalances["imported"]["locked"] // wallet.GetBalanceMinConfType("imported", 0, "locked")

	balances := &WalletBalances{
		AllAllAcounts:           balAllAll.ToCoin(),
		AllDefaultAcount:        balAllDefault.ToCoin(),
		SpendableAllAccounts:    balSpendableAll.ToCoin(),
		SpendableDefaultAccount: balSpendableDefault.ToCoin(),
		LockedAllAccounts:       balLockedAll.ToCoin(),
		LockedImportedAccount:   balLockedImported.ToCoin(),
		LockedDefaultAccount:    balLockedDefault.ToCoin(),
	}

	// Output
	winSize := uint32(activeNet.StakeDiffWindowSize)
	stakeinfo := &stakeInfoData{
		height:           height,
		walletInfo:       walletInfo,
		stakeinfo:        getStakeInfoRes,
		balances:         balances,
		accountBalances:  &accountBalances,
		priceWindowNum:   int(height / winSize),
		idxBlockInWindow: int(height%winSize) + 1,
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
	priceWindowNum   int
	idxBlockInWindow int
}

type blockDataCollector struct {
	mtx          sync.Mutex
	cfg          *config
	dcrdChainSvr *dcrrpcclient.Client
}

// newBlockDataCollector creates a new blockDataCollector.
func newBlockDataCollector(cfg *config,
	dcrdChainSvr *dcrrpcclient.Client) (*blockDataCollector, error) {
	return &blockDataCollector{
		mtx:          sync.Mutex{},
		cfg:          cfg,
		dcrdChainSvr: dcrdChainSvr,
	}, nil
}

// collect is the main handler for collecting chain data
func (t *blockDataCollector) collect(noTicketPool bool) (*blockData, error) {
	// In case of a very fast block, make sure previous call to collect is not
	// still running, or dcrd may be mad.
	t.mtx.Lock()
	defer t.mtx.Unlock()

	// Time this function
	defer func(start time.Time) {
		log.Tracef("blockDataCollector.collect() completed in %v", time.Since(start))
	}(time.Now())

	// Run first client call with a timeout
	type bbhRes struct {
		err  error
		hash *chainhash.Hash
	}
	toch := make(chan bbhRes)

	// Pull and store relevant data about the blockchain.
	go func() {
		bestBlockHash, err := t.dcrdChainSvr.GetBestBlockHash()
		toch <- bbhRes{err, bestBlockHash}
		return
	}()

	var bbs bbhRes
	select {
	case bbs = <-toch:
	case <-time.After(time.Second * 10):
		log.Errorf("Timeout waiting for dcrd.")
		return nil, errors.New("Timeout")
	}

	bestBlockHash := bbs.hash

	bestBlock, err := t.dcrdChainSvr.GetBlock(bestBlockHash)
	if err != nil {
		return nil, err
	}

	blockHeader := bestBlock.MsgBlock().Header
	//timestamp := blockHeader.Timestamp
	height := blockHeader.Height

	// In datasaver.go check TicketPoolInfo.PoolValue >= 0
	ticketPoolInfo := TicketPoolInfo{0, -1, -1}
	if !noTicketPool {
		poolSize := blockHeader.PoolSize

		poolValue, err := t.dcrdChainSvr.GetTicketPoolValue()
		if err != nil {
			return nil, err
		}
		avgPricePoolAmt := dcrutil.Amount(0)
		if poolSize != 0 {
			avgPricePoolAmt = poolValue / dcrutil.Amount(poolSize)
		}

		ticketPoolInfo = TicketPoolInfo{poolSize, poolValue.ToCoin(),
			avgPricePoolAmt.ToCoin()}
	}
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
	winSize := uint32(activeNet.StakeDiffWindowSize)
	blockdata := &blockData{
		header:           blockHeaderResults,
		feeinfo:          feeInfoBlock,
		currentstakediff: *stakeDiff,
		eststakediff:     *estStakeDiff,
		poolinfo:         ticketPoolInfo,
		priceWindowNum:   int(height / winSize),
		idxBlockInWindow: int(height%winSize) + 1,
	}

	return blockdata, err
}
