package main

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
	"github.com/decred/dcrwallet/wtxmgr"
)

// Arbitrary command execution
// cfg.CmdName // e.g. "ping"
// cfg.CmdArgs    // e.g. "127.0.0.1,-n-8"

// Define notification handlers
func getNodeNtfnHandlers(cfg *config) *dcrrpcclient.NotificationHandlers {
	return &dcrrpcclient.NotificationHandlers{
		OnBlockConnected: func(blockHeaderSerialized []byte, transactions [][]byte) {
			// OnBlockConnected: func(hash *chainhash.Hash, height int32,
			// 	time time.Time, vb uint16) {
			blockHeader := new(wire.BlockHeader)
			err := blockHeader.FromBytes(blockHeaderSerialized)
			if err != nil {
				log.Error("Failed to serialize blockHeader in new block notification.")
			}
			height := int32(blockHeader.Height)
			hash := blockHeader.BlockHash()
			select {
			case spyChans.connectChan <- &hash:
				// Past this point in this case is command execution. Block
				// height was sent on connectChan, so move on if no command.
				cmdName := cfg.CmdName
				if len(cmdName) == 0 {
					break
				}

				// replace %h and %n with hash and block height, resp.
				rep := strings.NewReplacer("%h", hash.String(), "%n",
					strconv.Itoa(int(height)))
				var argSubst bytes.Buffer
				rep.WriteString(&argSubst, cfg.CmdArgs)

				// Split the argument string by comma
				argsSplit := strings.Split(argSubst.String(), ",")

				// Create command, with substituted args
				cmd := exec.Command(cmdName, argsSplit...)
				// Get a pipe for stdout
				outpipe, err := cmd.StdoutPipe()
				if err != nil {
					log.Critical(err)
				}
				// Send stderr to the same place
				cmd.Stderr = cmd.Stdout

				// Display the full command being executed
				execLog.Debugf("Full command line to be executed: %s %s",
					cmd.Path, strings.Join(argsSplit, " "))

				// Channel for logger and command execution routines to talk
				cmdDone := make(chan error)
				go execLogger(outpipe, cmdDone)

				// Start command and return from handler without waiting
				go func() {
					if err := cmd.Run(); err != nil {
						execLog.Errorf("Failed to start system command %v. Error: %v",
							cmdName, err)
					}
					// Signal the logger goroutine, and clean up
					cmdDone <- err
					close(cmdDone)
				}()
			// send to nil channel blocks
			default:
			}

			// Also send on stake info channel, if enabled.
			select {
			case spyChans.connectChanStkInf <- height:
			// send to nil channel blocks
			default:
			}
		},
		// Not too useful since this notifies on every block
		OnStakeDifficulty: func(hash *chainhash.Hash, height int64,
			stakeDiff int64) {
			select {
			case spyChans.stakeDiffChan <- stakeDiff:
			default:
			}
		},
		// TODO
		OnWinningTickets: func(blockHash *chainhash.Hash, blockHeight int64,
			tickets []*chainhash.Hash) {
			var txstr []string
			for _, t := range tickets {
				txstr = append(txstr, t.String())
			}
			log.Debugf("Winning tickets: %v", strings.Join(txstr, ", "))
		},
		// maturing tickets
		// BUG: dcrrpcclient/notify.go (parseNewTicketsNtfnParams) is unable to
		// Unmarshal fourth parameter as a map[hash]hash.
		OnNewTickets: func(hash *chainhash.Hash, height int64, stakeDiff int64,
			tickets map[chainhash.Hash]chainhash.Hash) {
			for _, tick := range tickets {
				log.Debugf("Mined new ticket: %v", tick.String())
			}
		},
		// OnRelevantTxAccepted is invoked when a transaction containing a
		// registered address is inserted into mempool.
		OnRelevantTxAccepted: func(transaction []byte) {
			rec, err := wtxmgr.NewTxRecord(transaction, time.Now())
			if err != nil {
				return
			}
			tx := dcrutil.NewTx(&rec.MsgTx)
			txHash := rec.Hash
			select {
			case spyChans.relevantTxMempoolChan <- tx:
				log.Debugf("Detected transaction %v in mempool containing registered address.",
					txHash.String())
			default:
			}
		},
		// OnTxAccepted is invoked when a transaction is accepted into the
		// memory pool.  It will only be invoked if a preceding call to
		// NotifyNewTransactions with the verbose flag set to false has been
		// made to register for the notification and the function is non-nil.
		OnTxAccepted: func(hash *chainhash.Hash, amount dcrutil.Amount) {
			// Just send the tx hash and let the goroutine handle everything.
			select {
			case spyChans.newTxChan <- hash:
			default:
			}
			//log.Info("Transaction accepted to mempool: ", hash, amount)
		},
		// Note: dcrjson.TxRawResult is from getrawtransaction
		//OnTxAcceptedVerbose: func(txDetails *dcrjson.TxRawResult) {
		//txDetails.Hex
		//log.Info("Transaction accepted to mempool: ", txDetails.Txid)
		//},
	}
}

func getWalletNtfnHandlers(cfg *config) *dcrrpcclient.NotificationHandlers {
	return &dcrrpcclient.NotificationHandlers{
		OnAccountBalance: func(account string, balance dcrutil.Amount, confirmed bool) {
			log.Debug("OnAccountBalance")
		},
	}
}
