package main

import (
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrutil"
)

const (
	// blockConnChanBuffer is the size of the block connected channel buffer.
	blockConnChanBuffer = 8

	// newTxChanBuffer is the size of the new transaction channel buffer, for
	// ANY transactions are added into mempool.
	newTxChanBuffer = 2000

	// relevantMempoolTxChanBuffer is the size of the new transaction channel
	// buffer, for relevant transactions that are added into mempool.
	relevantMempoolTxChanBuffer = 512
)

// BlockWatchedTx contains, for a certain block, the transactions for certain
// watched addresses
type BlockWatchedTx struct {
	BlockHeight   int64
	TxsForAddress map[string][]*dcrutil.Tx
}

// Channels are package-level variables for simplicity
var spyChans struct {
	txTicker *time.Ticker

	connectChan                       chan *chainhash.Hash
	stakeDiffChan                     chan int64
	connectChanStkInf                 chan int32
	spendTxBlockChan, recvTxBlockChan chan *BlockWatchedTx
	relevantTxMempoolChan             chan *dcrutil.Tx
	newTxChan                         chan *chainhash.Hash
}

func makeChans(cfg *config) {
	// If we're monitoring for blocks OR collecting block data, these channels
	// are necessary to handle new block notifications. Otherwise, leave them
	// as nil so that both a send (below) blocks and a receive (in spy.go,
	// blockConnectedHandler) block. default case makes non-blocking below.
	// quit channel case manages blockConnectedHandlers.
	if !cfg.NoCollectBlockData && !cfg.NoMonitor {
		spyChans.connectChan = make(chan *chainhash.Hash, blockConnChanBuffer)
		spyChans.stakeDiffChan = make(chan int64, blockConnChanBuffer)
	}

	// Like connectChan for block data, connectChanStkInf is used when a new
	// block is connected, but to signal the stake info monitor.
	if !cfg.NoCollectStakeInfo && !cfg.NoMonitor {
		spyChans.connectChanStkInf = make(chan int32, blockConnChanBuffer)
	}

	// watchaddress
	if len(cfg.WatchAddresses) > 0 && !cfg.NoMonitor {
		// recv/spendTxBlockChan come with connected blocks
		spyChans.recvTxBlockChan = make(chan *BlockWatchedTx, blockConnChanBuffer)
		spyChans.spendTxBlockChan = make(chan *BlockWatchedTx, blockConnChanBuffer)
		spyChans.relevantTxMempoolChan = make(chan *dcrutil.Tx, relevantMempoolTxChanBuffer)
	}

	if cfg.MonitorMempool {
		spyChans.newTxChan = make(chan *chainhash.Hash, newTxChanBuffer)
	}
}

func closeChans() {
	if spyChans.stakeDiffChan != nil {
		close(spyChans.stakeDiffChan)
	}
	if spyChans.connectChan != nil {
		close(spyChans.connectChan)
	}
	if spyChans.connectChanStkInf != nil {
		close(spyChans.connectChanStkInf)
	}

	if spyChans.newTxChan != nil {
		spyChans.txTicker.Stop()
		close(spyChans.newTxChan)
	}
	if spyChans.relevantTxMempoolChan != nil {
		close(spyChans.relevantTxMempoolChan)
	}

	if spyChans.spendTxBlockChan != nil {
		close(spyChans.spendTxBlockChan)
	}
	if spyChans.recvTxBlockChan != nil {
		close(spyChans.recvTxBlockChan)
	}
}
