// Watch for transactions receiving to or sending from certain addresses.
// Receiving works, sending is probably messed up.
package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

// handleReceivingTx should be run as a go routine, and handles notification
// of transactions receiving to a registered address.  If no email notification
// is required, emailConf may be a nil pointer.  addrs is a map of addresses as
// strings with bool values indicating if email should be sent in response to
// transactions involving the keyed address.
func handleReceivingTx(c *dcrrpcclient.Client, addrs map[string]TxAction,
	emailConf *emailConfig, wg *sync.WaitGroup,
	quit <-chan struct{}) {
	defer wg.Done()
	//out:
	for {
	receive:
		select {
		// The message with all tx for watched addresses in new block
		case txsByAddr, ok := <-spyChans.recvTxBlockChan:
			// map[string][]*dcrutil.Tx is a map of addresses to slices of
			// transactions using that address.
			if !ok {
				log.Infof("Receive-Tx-in-block watch channel closed")
				return
			}
			if len(txsByAddr) == 0 {
				break receive
			}

			var height int64
			for _, txs := range txsByAddr {
				if len(txs) == 0 {
					break receive
				}
				txh := txs[0].Sha()
				txRes, err := c.GetRawTransactionVerbose(txh)
				if err != nil {
					log.Error("Unable to get raw transaction for", txh)
					continue
				}
				height = txRes.BlockHeight
				break
			}

			action := "mined into block"
			txAction := TxMined
			var recvStrings []string

			// For each address in map, process each tx
			for addr, txs := range txsByAddr {
				if len(txs) == 0 {
					continue
				}

				for _, tx := range txs {
					// Check the addresses associated with the PkScript of each TxOut
					for outID, txOut := range tx.MsgTx().TxOut {
						_, txAddrs, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
							txOut.PkScript, activeChain)
						if err != nil {
							log.Infof("ExtractPkScriptAddrs: %v", err.Error())
							// Next TxOut
							continue
						}

						// Check if this is a TxOut for the address
						for _, txAddr := range txAddrs {
							//log.Debug(addr, ", ", txAddr.EncodeAddress())
							if addr != txAddr.EncodeAddress() {
								// Next address for this TxOut
								continue
							}
							if addrActn, ok := addrs[addr]; ok {
								recvString := fmt.Sprintf(
									"Transaction sending to %v, value %.6f, "+
										"%s %d: %s[%d]",
									addr, dcrutil.Amount(txOut.Value).ToCoin(),
									action, height, tx.Sha().String(), outID)
								log.Infof(recvString)
								// Email notification if watchaddress has the ",1"
								// suffix AND we have a non-nil *emailConfig
								if (addrActn&txAction) > 0 && emailConf != nil {
									recvStrings = append(recvStrings, recvString)
								}
							}
						}
					}
				}
			}

			if len(recvStrings) > 0 && emailConf != nil {
				go sendEmailWatchRecv(strings.Join(recvStrings, "\n"), emailConf)
			}

		case tx, ok := <-spyChans.relevantTxMempoolChan:
			if !ok {
				log.Infof("Receive-Tx watch channel closed")
				return
			}

			// Make like notifyForTxOuts and screen the transactions TxOuts for
			// addresses we are watching for.
			_, height, err := c.GetBestBlock()
			if err != nil {
				log.Error("Unable to get best block.")
				break
			}

			// TODO also make this function handle mined tx again, with a
			// gettransaction to see if it's in a block
			action := "inserted into mempool"
			txAction := TxInserted

			// Check the addresses associated with the PkScript of each TxOut
			var recvStrings []string
			for _, txOut := range tx.MsgTx().TxOut {
				_, txAddrs, _, err := txscript.ExtractPkScriptAddrs(txOut.Version,
					txOut.PkScript, activeChain)
				if err != nil {
					log.Infof("ExtractPkScriptAddrs: %v", err.Error())
					continue
				}

				// Check if we are watching any address for this TxOut
				for _, txAddr := range txAddrs {
					addrstr := txAddr.EncodeAddress()
					if addrActn, ok := addrs[addrstr]; ok {
						recvString := fmt.Sprintf(
							"Transaction sending to %v, value %.6f, %v"+
								", after block %d: %s",
							addrstr, dcrutil.Amount(txOut.Value).ToCoin(),
							action, height, tx.Sha().String())
						log.Infof(recvString)
						// Email notification if watchaddress has the ",1"
						// suffix AND we have a non-nil *emailConfig
						if (addrActn&txAction) > 0 && emailConf != nil {
							recvStrings = append(recvStrings, recvString)
						}
						continue
					}
				}
			}

			if len(recvStrings) > 0 {
				go sendEmailWatchRecv(strings.Join(recvStrings, "\n"), emailConf)
			}

		case <-quit:
			mempoolLog.Debugf("Quitting OnRecvTx handler.")
			return
		}
	}

}

// handleSendingTx is DEAD

// Rather than watching for the sending address, which isn't known ahead of
// time, watch for a transaction with an input (source) whos previous outpoint
// is one of the watched addresses.
// But I am not sure we can do that here with the Tx and BlockDetails provided.
func handleSendingTx(c *dcrrpcclient.Client, addrs map[string]TxAction,
	spendTxChan <-chan *watchedAddrTx, wg *sync.WaitGroup,
	quit <-chan struct{}) {
	defer wg.Done()
	//out:
	for {
		//keepon:
		select {
		case addrTx, ok := <-spendTxChan:
			if !ok {
				log.Infof("Send Tx watch channel closed")
				return
			}

			// Unfortunately, can't make like notifyForTxOuts because we are
			// not watching outpoints.  For the tx we are given, we need to
			// search through each TxIn's PreviousOutPoints, requesting the raw
			// transaction from each PreviousOutPoint's tx hash, and check each
			// TxOut in the result for each watched address.  Phew! There is
			// surely a better way, but I don't know it.
			height, _, err := c.GetBestBlock()
			if err != nil {
				log.Error("Unable to get best block.")
				break
			}

			tx := addrTx.transaction
			var action string
			if addrTx.details != nil {
				action = fmt.Sprintf("mined into block %d.", height)
			} else {
				action = "inserted into mempool."
			}

			log.Debugf("Transaction with watched address as previous outpoint (spending) %s. Hash: %v",
				action, tx.Sha().String())

			for _, txIn := range tx.MsgTx().TxIn {
				prevOut := &txIn.PreviousOutPoint
				// uh, now what?
				// For each TxIn, we need to check the indicated vout index in the
				// txid of the previous outpoint.
				//txrr, err := c.GetRawTransactionVerbose(&prevOut.Hash)
				Tx, err := c.GetRawTransaction(&prevOut.Hash)
				if err != nil {
					log.Error("Unable to get raw transaction for", Tx)
					continue
				}

				// prevOut.Index should tell us which one, right?  Check all anyway.
				wireMsg := Tx.MsgTx()
				if wireMsg == nil {
					log.Debug("No wire Msg? Hmm.")
					continue
				}
				for _, txOut := range wireMsg.TxOut {
					_, txAddrs, _, err := txscript.ExtractPkScriptAddrs(
						txOut.Version, txOut.PkScript, activeChain)
					if err != nil {
						log.Infof("ExtractPkScriptAddrs: %v", err.Error())
						continue
					}

					for _, txAddr := range txAddrs {
						addrstr := txAddr.EncodeAddress()
						if _, ok := addrs[addrstr]; ok {
							log.Infof("Transaction with watched address %v as previous outpoint (spending), value %.6f, %v",
								addrstr, dcrutil.Amount(txOut.Value).ToCoin(), action)
							continue
						}
					}
				}

				// That's not what I'm doing here, but I'm looking anyway...
				// log.Debug(txscript.GetScriptClass(txscript.DefaultScriptVersion, txIn.SignatureScript))
				// log.Debug(txscript.GetPkScriptFromP2SHSigScript(txIn.SignatureScript))
				// sclass, txAddrs, nreqsigs, err := txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion, txIn.SignatureScript, activeChain)
				// log.Debug(sclass, txAddrs, nreqsigs, err, action)

				// addresses := make([]string, len(txAddrs))
				// for i, addr := range txAddrs {
				// 	addresses[i] = addr.EncodeAddress()
				// }
				// log.Debug(addresses)
			}
		case <-quit:
			mempoolLog.Debugf("Quitting OnRedeemingTx handler.")
			return
		}
	}
}

type watchedAddrTx struct {
	transaction *dcrutil.Tx
	details     *int
}
