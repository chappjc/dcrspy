package main

import (
	"fmt"
	"sync"

	_ "github.com/decred/dcrd/chaincfg/chainhash"
	_ "github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrrpcclient"
	_ "github.com/decred/dcrutil"
)

func handleReceivingTx(c *dcrrpcclient.Client, addrs map[string]struct{},
	recvTxChan <-chan *watchedAddrTx, wg *sync.WaitGroup,
	quit <-chan struct{}) {
	defer wg.Done()
	//out:
	for {
		//keepon:
		select {
		case addrTx, ok := <-recvTxChan:
			if !ok {
				log.Infof("Receive Tx watch channel closed")
				return
			}

			// Make like notifyForTxOuts and screen the transactions TxOuts for
			// addresses we are watching for.

			tx := addrTx.transaction
			var action string
			if addrTx.details != nil {
				action = fmt.Sprintf("mined into block %d.", addrTx.details.Height)
			} else {
				action = "inserted into mempool."
			}

			for _, txOut := range tx.MsgTx().TxOut {
				_, txAddrs, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
					txOut.PkScript, activeChain)
				if err != nil {
					log.Infof("ExtractPkScriptAddrs: %v",err.Error())
					continue
				}

				for _, txAddr := range txAddrs {
					addrstr := txAddr.EncodeAddress()
					if _, ok := addrs[addrstr]; ok {
						log.Infof("Transaction with watched address %v as outpoint %v",
							addrstr, action)
						continue
					}
				}
			}

		case <-quit:
			mempoolLog.Debugf("Quitting OnRecvTx/OnRedeemingTx handler.")
			return
		}
	}

}

// Rather than watching for the sending address, which isn't know ahead of
// time, watch for a transaction with an input (source) whos previous outpoint
// is one of the watched addresses.
// But I am not sure we can do that here with the Tx and BlockDetails provided.
func handleSendingTx(c *dcrrpcclient.Client, addrs map[string]struct{},
	addrTx *watchedAddrTx) {
	// Make like notifyForTxIns and screen the transactions TxIns for
	// addresses we are watching for.

	tx := addrTx.transaction
	var action string
	if addrTx.details != nil {
		action = fmt.Sprintf("mined into block %d.", addrTx.details.Height)
	} else {
		action = "inserted into mempool."
	}

	for _, txIn := range tx.MsgTx().TxIn {
		//prevOut := &txIn.PreviousOutPoint
		// uh, now what?
		// No way this is right, but I'm looking anyway...
		sclass, txAddrs, nreqsigs, err := txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion, txIn.SignatureScript, activeChain)
		log.Debug(sclass, txAddrs, nreqsigs, err, action)
	}
}
