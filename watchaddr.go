// Watch for transactions receiving to or sending from certain addresses.
// Receiving works, sending is probably messed up.
package main

import (
	"errors"
	"fmt"
	"net/smtp"
	//"strings"
	"strconv"
	"sync"

	_ "github.com/decred/dcrd/chaincfg/chainhash"
	_ "github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

type emailConfig struct {
	emailAddr                      string
	smtpUser, smtpPass, smtpServer string
	smtpPort                       int
}

// TxAction is what is happening to the transaction (mined or inserted into
// mempool).
type TxAction int32

// Valid values for TxAction
const (
	TxMined TxAction = 1 << iota
	TxInserted
	// removed? invalidated?
)

// sendEmailWatchRecv Sends an email using the input emailConfig and message
// string.
func sendEmailWatchRecv(message string, ecfg *emailConfig) error {
	// Check for nil pointer emailConfig
	if ecfg == nil {
		return errors.New("emailConfig must not be a nil pointer")
	}

	auth := smtp.PlainAuth(
		"",
		ecfg.smtpUser,
		ecfg.smtpPass,
		ecfg.smtpServer,
	)

	// The SMTP server address includes the port
	addr := ecfg.smtpServer + ":" + strconv.Itoa(ecfg.smtpPort)
	//log.Debug(addr)

	// Make a header using a map for clarity
	header := make(map[string]string)
	header["From"] = ecfg.smtpUser
	header["To"] = ecfg.emailAddr
	// TODO: make subject line adjustable or include an amount
	header["Subject"] = "dcrspy notification"
	//header["MIME-Version"] = "1.0"
	//header["Content-Type"] = "text/plain; charset=\"utf-8\""
	//header["Content-Transfer-Encoding"] = "base64"

	// Build the full message with the header + input message string
	messageFull := ""
	for k, v := range header {
		messageFull += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	messageFull += "\r\n" + message

	// Send email
	err := smtp.SendMail(
		addr,
		auth,
		ecfg.smtpUser,            // sender is receiver
		[]string{ecfg.emailAddr}, // recipients
		[]byte(messageFull),
	)

	if err != nil {
		log.Errorf("Failed to send email: %v", err)
		return err
	}

	log.Tracef("Send email to address %v\n", ecfg.emailAddr)
	return nil
}

// handleReceivingTx should be run as a go routine, and handles notification
// of transactions receiving to a registered address.  If no email notification
// is requried, emailConf may be a nil pointer.  addrs is a map of addresses as
// strings with bool values indicating if email should be sent in response to
// transactions involving the keyed address.
func handleReceivingTx(c *dcrrpcclient.Client, addrs map[string]TxAction,
	emailConf *emailConfig,
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
			var txAction TxAction
			if addrTx.details != nil {
				action = fmt.Sprintf("mined into block %d.", addrTx.details.Height)
				txAction = TxMined
			} else {
				action = "inserted into mempool."
				txAction = TxInserted
			}

			// Check the addresses associated with the PkScript of each TxOut
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
						recvString := fmt.Sprintf("Transaction with watched address %v as outpoint (receiving), value %.6f, %v",
							addrstr, dcrutil.Amount(txOut.Value).ToCoin(),
							action)
						log.Infof(recvString)
						// Email notification if watchaddress has the ",1"
						// suffix AND we have a non-nil *emailConfig
						if (addrActn&txAction) > 0 && emailConf != nil {
							go sendEmailWatchRecv(recvString, emailConf)
						}
						continue
					}
				}
			}

		case <-quit:
			mempoolLog.Debugf("Quitting OnRecvTx handler.")
			return
		}
	}

}

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

			tx := addrTx.transaction
			var action string
			if addrTx.details != nil {
				action = fmt.Sprintf("mined into block %d.", addrTx.details.Height)
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
