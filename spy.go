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
)

// for getblock, ticketfeeinfo, estimatestakediff, etc.
type chainMonitor struct {
	collector    *blockDataCollector
	dataSavers   []BlockDataSaver
	quit         chan struct{}
	wg           *sync.WaitGroup
	noTicketPool bool
	watchaddrs   map[string]TxAction
}

// newChainMonitor creates a new chainMonitor
func newChainMonitor(collector *blockDataCollector,
	savers []BlockDataSaver,
	quit chan struct{}, wg *sync.WaitGroup, noPoolValue bool,
	addrs map[string]TxAction) *chainMonitor {
	return &chainMonitor{
		collector:    collector,
		dataSavers:   savers,
		quit:         quit,
		wg:           wg,
		noTicketPool: noPoolValue,
		watchaddrs:   addrs,
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
		case hash, ok := <-spyChans.connectChan:
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
				// 	p.spendTxBlockChan <- &BlockWatchedTx{height, txsForOutpoints}
				// }

				txsForAddrs := BlockReceivesToAddresses(block, p.watchaddrs)
				if len(txsForAddrs) > 0 {
					spyChans.recvTxBlockChan <- &BlockWatchedTx{height,
						txsForAddrs}
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
	collector  *stakeInfoDataCollector
	dataSavers []StakeInfoDataSaver
	quit       chan struct{}
	wg         *sync.WaitGroup
}

// newStakeMonitor creates a new stakeMonitor
func newStakeMonitor(collector *stakeInfoDataCollector,
	savers []StakeInfoDataSaver,
	quit chan struct{}, wg *sync.WaitGroup) *stakeMonitor {
	return &stakeMonitor{
		collector:  collector,
		dataSavers: savers,
		quit:       quit,
		wg:         wg,
	}
}

// blockConnectedHandler handles block connected notifications, which trigger
// data collection and storage.
func (p *stakeMonitor) blockConnectedHandler() {
	defer p.wg.Done()
out:
	for {
		select {
		case height, ok := <-spyChans.connectChanStkInf:
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
