// spy.go defines the chainMonitor and stakeMonitor, which handle the block
// connected notifications via blockConnChan.  They are separate because we
// might want to run without a wallet, just monitoring dcrd data.
//
// chappjc

package main

import (
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

func txhashInSlice(txs []*dcrutil.Tx, txHash *chainhash.Hash) *dcrutil.Tx {
	if len(txs) < 1 {
		return nil
	}

	for _, minedTx := range txs {
		txSha := minedTx.Sha()
		if txHash.IsEqual(txSha) {
			return minedTx
		}
	}
	return nil
}

// includesTx checks if a block contains a transaction hash
func includesStakeTx(txHash *chainhash.Hash, block *dcrutil.Block) (int, int8) {
	blockTxs := block.STransactions()

	if tx := txhashInSlice(blockTxs, txHash); tx != nil {
		return tx.Index(), tx.Tree()
	}
	return -1, -1
}

// includesTx checks if a block contains a transaction hash
func includesTx(txHash *chainhash.Hash, block *dcrutil.Block) (int, int8) {
	blockTxs := block.Transactions()

	if tx := txhashInSlice(blockTxs, txHash); tx != nil {
		return tx.Index(), tx.Tree()
	}
	return -1, -1
}

func blockConsumesOutpointWithAddresses(block *dcrutil.Block, addrs map[string]TxAction,
	c *dcrrpcclient.Client) map[string][]*dcrutil.Tx {
	addrMap := make(map[string][]*dcrutil.Tx)

	checkForOutpointAddr := func(blockTxs []*dcrutil.Tx) {
		for _, tx := range blockTxs {
			for _, txIn := range tx.MsgTx().TxIn {
				prevOut := &txIn.PreviousOutPoint
				// For each TxIn, check the indicated vout index in the txid of the
				// previous outpoint.
				// txrr, err := c.GetRawTransactionVerbose(&prevOut.Hash)
				prevTx, err := c.GetRawTransaction(&prevOut.Hash)
				if err != nil {
					log.Debug("Unable to get raw transaction for ", prevOut.Hash.String())
					continue
				}

				// prevOut.Index should tell us which one, but check all anyway
				for _, txOut := range prevTx.MsgTx().TxOut {
					_, txAddrs, _, err := txscript.ExtractPkScriptAddrs(
						txOut.Version, txOut.PkScript, activeChain)
					if err != nil {
						log.Infof("ExtractPkScriptAddrs: %v", err.Error())
						continue
					}

					for _, txAddr := range txAddrs {
						addrstr := txAddr.EncodeAddress()
						if _, ok := addrs[addrstr]; ok {
							if addrMap[addrstr] == nil {
								addrMap[addrstr] = make([]*dcrutil.Tx, 0)
							}
							addrMap[addrstr] = append(addrMap[addrstr], prevTx)
						}
					}
				}
			}
		}
	}

	checkForOutpointAddr(block.Transactions())
	checkForOutpointAddr(block.STransactions())

	return addrMap
}

func blockReceivesToAddresses(block *dcrutil.Block, addrs map[string]TxAction,
	c *dcrrpcclient.Client) map[string][]*dcrutil.Tx {
	addrMap := make(map[string][]*dcrutil.Tx)

	checkForAddrOut := func(blockTxs []*dcrutil.Tx) {
		for _, tx := range blockTxs {
			// Check the addresses associated with the PkScript of each TxOut
			for _, txOut := range tx.MsgTx().TxOut {
				_, txOutAddrs, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
					txOut.PkScript, activeChain)
				if err != nil {
					log.Infof("ExtractPkScriptAddrs: %v", err.Error())
					continue
				}

				// Check if we are watching any address for this TxOut
				for _, txAddr := range txOutAddrs {
					addrstr := txAddr.EncodeAddress()
					if _, ok := addrs[addrstr]; ok {
						if _, gotSlice := addrMap[addrstr]; !gotSlice {
							addrMap[addrstr] = make([]*dcrutil.Tx, 0)
						}
						//log.Info("Receives to: ", addrstr)
						addrMap[addrstr] = append(addrMap[addrstr], tx)
					}
				}
			}
		}
	}

	checkForAddrOut(block.Transactions())
	checkForAddrOut(block.STransactions())

	return addrMap
}

// for getblock, ticketfeeinfo, estimatestakediff, etc.
type chainMonitor struct {
	collector          *blockDataCollector
	dataSavers         []BlockDataSaver
	blockConnectedChan chan *chainhash.Hash
	quit               chan struct{}
	wg                 *sync.WaitGroup
	noTicketPool       bool
	watchaddrs         map[string]TxAction
	spendTxBlockChan   chan map[string][]*dcrutil.Tx
	recvTxBlockChan    chan map[string][]*dcrutil.Tx
}

// newChainMonitor creates a new chainMonitor
func newChainMonitor(collector *blockDataCollector,
	blockConnChan chan *chainhash.Hash, savers []BlockDataSaver,
	quit chan struct{}, wg *sync.WaitGroup, noPoolValue bool,
	addrs map[string]TxAction,
	spendTxBlockChan chan map[string][]*dcrutil.Tx,
	recvTxBlockChan chan map[string][]*dcrutil.Tx) *chainMonitor {
	return &chainMonitor{
		collector:          collector,
		dataSavers:         savers,
		blockConnectedChan: blockConnChan,
		quit:               quit,
		wg:                 wg,
		noTicketPool:       noPoolValue,
		watchaddrs:         addrs,
		spendTxBlockChan:   spendTxBlockChan,
		recvTxBlockChan:    recvTxBlockChan,
	}
}

// blockConnectedHandler handles block connected notifications, which trigger
// data collection and storage.
func (p *chainMonitor) blockConnectedHandler() {
	defer p.wg.Done()
out:
	for {
	keepon:
		select {
		case hash, ok := <-p.blockConnectedChan:
			if !ok {
				log.Warnf("Block connected channel closed.")
				break out
			}
			block, _ := p.collector.dcrdChainSvr.GetBlock(hash)
			height := block.Height()
			daemonLog.Infof("Block height %v connected", height)

			if len(p.watchaddrs) > 0 {
				// txsForOutpoints := blockConsumesOutpointWithAddresses(block, p.watchaddrs,
				// 	p.collector.dcrdChainSvr)
				// if len(txsForOutpoints) > 0 {
				// 	p.spendTxBlockChan <- txsForOutpoints
				// }

				txsForAddrs := blockReceivesToAddresses(block, p.watchaddrs,
					p.collector.dcrdChainSvr)
				if len(txsForAddrs) > 0 {
					p.recvTxBlockChan <- txsForAddrs
				}
			}

			// data collection with timeout
			bdataChan := make(chan *blockData)
			// fire it off and get the blockData pointer back through the channel
			go func() {
				BlockData, err := p.collector.collect(p.noTicketPool)
				if err != nil {
					log.Errorf("Block data collection failed: %v", err.Error())
					// BlockData is nil when err != nil
				}
				bdataChan <- BlockData
			}()

			// Wait for X seconds before giving up on collect()
			var BlockData *blockData
			select {
			case BlockData = <-bdataChan:
				if BlockData == nil {
					break keepon
				}
			case <-time.After(time.Second * 20):
				log.Errorf("Block data collection TIMEOUT after 20 seconds.")
				break keepon
			}

			// Store block data with each saver
			for _, s := range p.dataSavers {
				if s != nil {
					// save data to wherever the saver wants to put it
					go s.Store(BlockData)
				}
			}

		case _, ok := <-p.quit:
			if !ok {
				log.Debugf("Got quit signal. Exiting block connected handler for BLOCK monitor.")
				break out
			}
		}
	}

}

// for getstakeinfo, etc.
type stakeMonitor struct {
	collector          *stakeInfoDataCollector
	dataSavers         []StakeInfoDataSaver
	blockConnectedChan chan int32
	quit               chan struct{}
	wg                 *sync.WaitGroup
}

// newStakeMonitor creates a new stakeMonitor
func newStakeMonitor(collector *stakeInfoDataCollector,
	blockConnChan chan int32, savers []StakeInfoDataSaver,
	quit chan struct{}, wg *sync.WaitGroup) *stakeMonitor {
	return &stakeMonitor{
		collector:          collector,
		dataSavers:         savers,
		blockConnectedChan: blockConnChan,
		quit:               quit,
		wg:                 wg,
	}
}

// blockConnectedHandler handles block connected notifications, which trigger
// data collection and storage.
func (p *stakeMonitor) blockConnectedHandler() {
	defer p.wg.Done()
out:
	for {
		select {
		case height, ok := <-p.blockConnectedChan:
			if !ok {
				log.Warnf("Block connected channel closed.")
				break out
			}

			// Let the wallet process the new block (too bad no wallet ntfns!)
			time.Sleep(time.Millisecond * 300)

			// Try to collect the data, retry if wallet says to
		collect:
			stakeInfo, err := p.collector.collect(uint32(height))
			if err != nil {
				log.Errorf("Stake info data collection failed: %v", err)
				// Look for that -4 message from wallet that says: "the wallet is
				// currently syncing to the best block, please try again later"
				if strings.Contains(err.Error(), "try again later") {
					time.Sleep(time.Millisecond * 700)
					goto collect // mmm, feel so dirty! maybe make this "cleaner" later
				}
				break out
			}

			for _, s := range p.dataSavers {
				if s != nil {
					// save data to wherever the saver wants to put it
					go s.Store(stakeInfo)
				}
			}

		case _, ok := <-p.quit:
			if !ok {
				log.Debugf("Got quit signal. Exiting block connected handler for STAKE monitor.")
				break out
			}
		}
	}

}
