// DCRSPY is a program to continuously monitor and log changes in various data
// on the Decred network.  It works by connecting to both dcrd and dcrwallet,
// and responding when a new block is detected via a notifier registered with
// dcrd.
//
// Two types of information are monitored:
//	1. Block chain data (from dcrd)
//  2. Stake information (from your wallet)
//
// See README.md and TODO for more information.
//
// by Jonathan Chappelow (chappjc)
//
// Borrowing logging and config file facilities, plus much boilerplate from
// main.go, from on dcrticketbuyer, by the Decred developers.

package main

import (
	"bufio"
	"bytes"
	_ "encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	//"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

type watchedAddrTx struct {
	transaction *dcrutil.Tx
	details     *dcrjson.BlockDetails
}

const (
	// blockConnChanBuffer is the size of the block connected channel buffer.
	blockConnChanBuffer = 100
)

func main() {
	// Parse the configuration file.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Failed to load dcrspy config: %s\n", err.Error())
		os.Exit(1)
	}
	defer backendLog.Flush()

	dcrrpcclient.UseLogger(clientLog)

	log.Infof(appName+" version %s", ver.String())

	// Create data output folder if it does not already exist
	err = os.MkdirAll(cfg.OutFolder, 0755)
	if err != nil {
		fmt.Printf("Failed to create data output folder %s. Error: %s\n",
			cfg.OutFolder, err.Error())
		os.Exit(1)
	}

	// Connect to dcrd RPC server using websockets. Set up the
	// notification handler to deliver blocks through a channel.
	var connectChan chan int32
	var stakeDiffChan chan int64
	// If we're monitoring for blocks OR collecting block data, these channels
	// are necessary to handle new block notifications. Otherwise, leave them
	// as nil so that both a send (below) blocks and a receive (in spy.go,
	// blockConnectedHandler) block. default case makes non-blocking below.
	// quit channel case manages blockConnectedHandlers.
	if !cfg.NoCollectBlockData && !cfg.NoMonitor {
		connectChan = make(chan int32, blockConnChanBuffer)
		stakeDiffChan = make(chan int64, 2)
	}

	// mempool: new transactions, new tickets
	//cfg.MonitorMempool = cfg.MonitorMempool && !cfg.NoMonitor
	if cfg.MonitorMempool && cfg.NoMonitor {
		log.Warn("Both --nomonitor (-e) and --mempool (-m) specified.  Not monitoring mempool.")
		cfg.MonitorMempool = false
	}

	var newTxChan chan *chainhash.Hash
	if cfg.MonitorMempool {
		newTxChan = make(chan *chainhash.Hash, 80)
	}

	// watchaddress
	var recvTxChan, spendTxChan chan *watchedAddrTx
	if len(cfg.WatchAddresses) > 0  && !cfg.NoMonitor {
		recvTxChan = make(chan *watchedAddrTx, 80)
		spendTxChan = make(chan *watchedAddrTx, 80)
	}

	var connectChanStkInf chan int32
	if !cfg.NoCollectStakeInfo && !cfg.NoMonitor {
		connectChanStkInf = make(chan int32, blockConnChanBuffer)
	}

	cmdName := cfg.CmdName // e.g. "ping"
	args := cfg.CmdArgs    // e.g. "127.0.0.1,-n-8"

	ntfnHandlersDaemon := dcrrpcclient.NotificationHandlers{
		OnBlockConnected: func(hash *chainhash.Hash, height int32,
			time time.Time, vb uint16) {
			select {
			case connectChan <- height:
				// Past this point in this case is command execution
				if len(cmdName) == 0 {
					break
				}

				// replace %h and %n with hash and block height, resp.
				rep := strings.NewReplacer("%h", hash.String(), "%n", strconv.Itoa(int(height)))
				var argSubst bytes.Buffer
				rep.WriteString(&argSubst, args)

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
				execLog.Infof("Full command line to be executed: %s %s",
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
			select {
			case connectChanStkInf <- height:
			// send to nil channel blocks
			default:
			}
		},
		// BUG: This appears to be currently broken, notifiying on every block.
		OnStakeDifficulty: func(hash *chainhash.Hash, height int64,
			stakeDiff int64) {
			select {
			case stakeDiffChan <- stakeDiff:
			default:
			}
		},
		//
		OnWinningTickets: func(blockHash *chainhash.Hash, blockHeight int64,
			tickets []*chainhash.Hash) {
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
		// OnRecvTx is invoked when a transaction that receives funds to a
		// registered address is received into the memory pool and also
		// connected to the longest (best) chain.
		OnRecvTx: func(transaction *dcrutil.Tx, details *dcrjson.BlockDetails) {
			// To determine if this was mined into a block or accepted into
			// mempool, we need details, which is nil in the mempool case
			wAddrTx := &watchedAddrTx{transaction, details}
			select {
			case recvTxChan <- wAddrTx:
				log.Infof("Detected transaction %v receiving funds into registered address.",
					transaction.Sha().String())
			default:
			}
		},
		// spend from registered address
		OnRedeemingTx: func(transaction *dcrutil.Tx, details *dcrjson.BlockDetails) {
			wAddrTx := &watchedAddrTx{transaction, details}
			select {
			case spendTxChan <- wAddrTx:
				log.Infof("Detected transaction %v spending funds from registered address.",
					transaction.Sha().String())
			default:
			}
		},
		// OnTxAccepted is invoked when a transaction is accepted into the
		// memory pool.  It will only be invoked if a preceding call to
		// NotifyNewTransactions with the verbose flag set to false has been
		// made to register for the notification and the function is non-nil.
		OnTxAccepted: func(hash *chainhash.Hash, amount dcrutil.Amount) {
			// b, err := hex.DecodeString(hash.String())
			// if err != nil {
			// 	log.Errorf("Unable to decode Tx hash string: %v, %v",hash.String(),
			// 		err.Error())
			// }
			// tx, err := dcrutil.NewTxFromBytes(hash.Bytes())
			// if err != nil {
			// 	log.Errorf("Unable to create Tx from bytes: %v, %v",hash.String(),
			// 		err.Error())
			// 	return
			// }
			select {
			case newTxChan <- hash:
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

	// Daemon client connection

	// dcrd rpc.cert
	var dcrdCerts []byte
	if !cfg.DisableClientTLS {
		dcrdCerts, err = ioutil.ReadFile(cfg.DcrdCert)
		if err != nil {
			fmt.Printf("Failed to read dcrd cert file at %s: %s\n", cfg.DcrdCert,
				err.Error())
			os.Exit(1)
		}
	}

	log.Debugf("Attempting to connect to dcrd RPC %s as user %s "+
		"using certificate located in %s",
		cfg.DcrdServ, cfg.DcrdUser, cfg.DcrdCert)

	connCfgDaemon := &dcrrpcclient.ConnConfig{
		Host:         cfg.DcrdServ,
		Endpoint:     "ws",
		User:         cfg.DcrdUser,
		Pass:         cfg.DcrdPass,
		Certificates: dcrdCerts,
		DisableTLS:   cfg.DisableClientTLS,
	}

	dcrdClient, err := dcrrpcclient.New(connCfgDaemon, &ntfnHandlersDaemon)
	if err != nil {
		fmt.Printf("Failed to start dcrd RPC client: %s\n", err.Error())
		os.Exit(1)
	}

	// Display connected network
	net, err := dcrdClient.GetCurrentNet()
	if err != nil {
		fmt.Printf("Unable to get current network from dcrd: %s\n", err.Error())
		os.Exit(1)
	}
	log.Infof("Connected to dcrd on network: %v", net.String())

	// Validate watchaddresses
	addresses := make([]dcrutil.Address, 0, len(cfg.WatchAddresses))
	addrMap := make(map[string]struct{})
	if len(cfg.WatchAddresses) > 0 && !cfg.NoMonitor {
		for _, a := range cfg.WatchAddresses {
			addr, err := dcrutil.DecodeAddress(a, activeNet.Params)
			// or DecodeNetworkAddress for auto-detection of network
			if err != nil {
				log.Errorf("Invalid watchaddress %v", a)
				os.Exit(1)
			}
			log.Infof("Valid watchaddress: %v", addr)
			addresses = append(addresses, addr)
			addrMap[a] = struct{}{}
		}
		if len(addresses) == 0 {
			if recvTxChan != nil {
				close(recvTxChan)
				recvTxChan = nil
			}
			if spendTxChan != nil {
				close(spendTxChan)
				spendTxChan = nil
			}
		}
	}

	// Register for block connection notifications.
	if err := dcrdClient.NotifyBlocks(); err != nil {
		fmt.Printf("Failed to register daemon RPC client for  "+
			"block notifications: %s\n", err.Error())
		os.Exit(1)
	}

	// Register for stake difficulty change notifications.
	if err := dcrdClient.NotifyStakeDifficulty(); err != nil {
		fmt.Printf("Failed to register daemon RPC client for  "+
			"stake difficulty change notifications: %s\n", err.Error())
		os.Exit(1)
	}

	// Register for tx accepted into mempool ntfns
	if err := dcrdClient.NotifyNewTransactions(false); err != nil {
		fmt.Printf("Failed to register daemon RPC client for  "+
			"new transaction (mempool) notifications: %s\n", err.Error())
		os.Exit(1)
	}

	// For OnNewTickets
	//  Commented since there is a bug in dcrrpcclient/notify.go
	// if err := dcrdClient.NotifyNewTickets(); err != nil {
	// 	fmt.Printf("Failed to register daemon RPC client for  "+
	// 		"new tickets (mempool) notifications: %s\n", err.Error())
	// 	os.Exit(1)
	// }

	// For OnRedeemingTx (spend) and OnRecvTx
	if len(addresses) > 0 {
		if err := dcrdClient.NotifyReceived(addresses); err != nil {
			fmt.Printf("Failed to register addresses.  Error: %v", err.Error())
			os.Exit(1)
		}
	}

	// Wallet

	var dcrwClient *dcrrpcclient.Client
	if !cfg.NoCollectStakeInfo {
		// dcrwallet rpc.cert
		var dcrwCerts []byte
		if !cfg.DisableClientTLS {
			dcrwCerts, err = ioutil.ReadFile(cfg.DcrwCert)
			if err != nil {
				log.Errorf("Failed to read dcrwallet cert file at %s: %s\n",
					cfg.DcrwCert, err.Error())
				// but try anyway?
			}
		}

		log.Debugf("Attempting to connect to dcrwallet RPC %s as user %s "+
			"using certificate located in %s",
			cfg.DcrwServ, cfg.DcrwUser, cfg.DcrwCert)

		connCfgWallet := &dcrrpcclient.ConnConfig{
			Host:         cfg.DcrwServ,
			Endpoint:     "ws",
			User:         cfg.DcrwUser,
			Pass:         cfg.DcrwPass,
			Certificates: dcrwCerts,
			DisableTLS:   cfg.DisableClientTLS,
		}

		dcrwClient, err = dcrrpcclient.New(connCfgWallet, nil)
		if err != nil {
			fmt.Printf("Failed to start dcrwallet RPC client: %s\nPerhaps you"+
				" wanted to start with --nostakeinfo?\n", err.Error())
			fmt.Printf("Verify that rpc.cert is for your wallet:\n\t%v", cfg.DcrwCert)
			os.Exit(1)
		}
	}

	// Ctrl-C to shut down.
	// Nothing should be sent the quit channel.  It should only be closed.
	quit := make(chan struct{})
	// Only accept a single CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Start waiting for the interrupt signal
	go func() {
		for range c {
			signal.Stop(c)
			// Close the channel so multiple goroutines can get the message
			log.Infof("CTRL+C hit.  Closing goroutines.")
			close(quit)
			return
		}
	}()

	// WaitGroup for the chainMonitor and stakeMonitor
	var wg sync.WaitGroup

	// Saver mutex, to share the same underlying output resource between block
	// and stake info data savers
	saverMutex := new(sync.Mutex)

	// Build a slice of each required saver type for each data source
	var blockDataSavers []BlockDataSaver
	var stakeInfoDataSavers []StakeInfoDataSaver
	var mempoolSavers []MempoolDataSaver
	// JSON to stdout
	if cfg.SaveJSONStdout {
		blockDataSavers = append(blockDataSavers, NewBlockDataToJSONStdOut(saverMutex))
		stakeInfoDataSavers = append(stakeInfoDataSavers, NewStakeInfoDataToJSONStdOut(saverMutex))
		mempoolSavers = append(mempoolSavers, NewMempoolDataToJSONStdOut(saverMutex))
	}
	// JSON to file
	if cfg.SaveJSONFile {
		blockDataSavers = append(blockDataSavers,
			NewBlockDataToJSONFiles(cfg.OutFolder, "block_data-", saverMutex))
		stakeInfoDataSavers = append(stakeInfoDataSavers,
			NewStakeInfoDataToJSONFiles(cfg.OutFolder, "stake-info-", saverMutex))
		mempoolSavers = append(mempoolSavers,
			NewMempoolDataToJSONFiles(cfg.OutFolder, "mempool-info-", saverMutex))
	}

	// If no savers specified, enable Summary Output
	if len(blockDataSavers) == 0 {
		cfg.SummaryOut = true
	}

	summarySaverBlockData := NewBlockDataToSummaryStdOut(saverMutex)
	summarySaverStakeInfo := NewStakeInfoDataToSummaryStdOut(saverMutex)
	summarySaverMempool := NewMempoolDataToSummaryStdOut(saverMutex)

	if cfg.SummaryOut {
		blockDataSavers = append(blockDataSavers, summarySaverBlockData)
		stakeInfoDataSavers = append(stakeInfoDataSavers, summarySaverStakeInfo)
		mempoolSavers = append(mempoolSavers, summarySaverMempool)
	}

	// Block data collector
	collector, err := newBlockDataCollector(cfg, dcrdClient)
	if err != nil {
		fmt.Printf("Failed to create block data collector: %s\n", err.Error())
		os.Exit(1)
	}

	backendLog.Flush()

	// Initial data summary prior to start of regular collection
	blockData, err := collector.collect(!cfg.PoolValue)
	if err != nil {
		fmt.Printf("Block data collection for initial summary failed. Error: %v", err.Error())
		os.Exit(1)
	}

	if err := summarySaverBlockData.Store(blockData); err != nil {
		fmt.Printf("Failed to print initial block data summary. Error: %v", err.Error())
		os.Exit(1)
	}

	if !cfg.NoCollectBlockData && !cfg.NoMonitor {
		// Blockchain monitor for the collector
		wg.Add(1)
		// If collector is nil, so is connectChan
		wsChainMonitor := newChainMonitor(collector, connectChan,
			blockDataSavers, quit, &wg, !cfg.PoolValue)
		go wsChainMonitor.blockConnectedHandler()
	}

	// Stake info data (getstakeinfo) collector
	var stakeCollector *stakeInfoDataCollector
	if !cfg.NoCollectStakeInfo {
		stakeCollector, err = newStakeInfoDataCollector(cfg, dcrdClient, dcrwClient)
		if err != nil {
			fmt.Printf("Failed to create block data collector: %s\n", err.Error())
			os.Exit(1)
		}

		// Initial data summary prior to start of regular collection
		height, err := stakeCollector.getHeight()
		if err != nil {
			fmt.Printf("Unable to get current block height. Error: %v", err.Error())
			os.Exit(1)
		}
		stakeInfoData, err := stakeCollector.collect(height)
		if err != nil {
			fmt.Printf("Stake info data collection failed gathering initial data. Error: %v", err.Error())
			os.Exit(1)
		}

		if err := summarySaverStakeInfo.Store(stakeInfoData); err != nil {
			fmt.Printf("Failed to print initial stake info data summary. Error: %v", err.Error())
			os.Exit(1)
		}

		if !cfg.NoMonitor {
			wg.Add(1)
			// Stake info monitor for the stakeCollector
			wsStakeInfoMonitor := newStakeMonitor(stakeCollector, connectChanStkInf,
				stakeInfoDataSavers, quit, &wg)
			go wsStakeInfoMonitor.blockConnectedHandler()
		}
	}

	var txTicker *time.Ticker
	if cfg.MonitorMempool {
		mpoolCollector, err := newMempoolDataCollector(cfg, dcrdClient)
		if err != nil {
			fmt.Printf("Failed to create mempool data collector: %s\n", err.Error())
			os.Exit(1)
		}

		mempoolInfo, err := mpoolCollector.collect()
		if err != nil {
			fmt.Printf("Mempool info collection failed while gathering initial data. Error: %v", err.Error())
			os.Exit(1)
		}

		if err := summarySaverMempool.Store(mempoolInfo); err != nil {
			fmt.Printf("Failed to print initial mempool info summary. Error: %v", err.Error())
			os.Exit(1)
		}

		newTicketLimit := int32(cfg.MPTriggerTickets)
		mini := time.Duration(time.Duration(cfg.MempoolMinInterval) * time.Second)
		maxi := time.Duration(time.Duration(cfg.MempoolMaxInterval) * time.Second)

		wg.Add(1)
		mpm := newMempoolMonitor(mpoolCollector, newTxChan, mempoolSavers,
			quit, &wg, newTicketLimit, mini, maxi)
		go mpm.txHandler(dcrdClient)

		txTicker = time.NewTicker(time.Second * 2)
		go func() {
			for range txTicker.C {
				newTxChan <- new(chainhash.Hash)
			}
		}()
	}

	if len(addresses) > 0 {
		wg.Add(2)
		go handleReceivingTx(dcrdClient, addrMap, recvTxChan, &wg, quit)
		go handleSendingTx(dcrdClient, addrMap, spendTxChan, &wg, quit)
	}

	// stakediff not implemented yet as the notifier appears broken
	go func() {
		for {
			select {
			case s, ok := <-stakeDiffChan:
				if !ok {
					log.Debugf("Stake difficulty channel closed")
					return
				}
				log.Debugf("Got stake difficulty change notification (%v). "+
					" Doing nothing for now.", s)
			case <-quit:
				log.Debugf("Quitting OnStakeDifficulty handler.")
				return
			}
		}
	}()

	log.Infof("RPC client(s) successfully connected. Now monitoring and " +
		"collecting data.")

	// When os.Interrupt arrives, the channel quit will be closed
	//<-quit
	wg.Wait()

	if cfg.NoMonitor {
		c <- os.Interrupt
		time.Sleep(200)
	}

	// Closing these channels should be unnecessary if quit was handled right
	if stakeDiffChan != nil {
		close(stakeDiffChan)
	}
	if connectChan != nil {
		close(connectChan)
	}
	if connectChanStkInf != nil {
		close(connectChanStkInf)
	}
	if newTxChan != nil {
		txTicker.Stop()
		close(newTxChan)
	}

	if recvTxChan != nil {
		close(recvTxChan)
	}
	if spendTxChan != nil {
		close(spendTxChan)
	}

	log.Infof("Closing connection to dcrd.")
	dcrdClient.Shutdown()

	if !cfg.NoCollectStakeInfo {
		log.Infof("Closing connection to dcrwallet.")
		dcrwClient.Shutdown()
	}

	log.Infof("Bye!")
	time.Sleep(500 * time.Millisecond)
	os.Exit(1)
}

// execLogger conitnually scans for new lines on outpipe, reads the text for
// each line and writes it to execlogger (i.e. EXEC).  This should be run as
// a goroutine, using the cmdDone receiving channel to signal to stop logging,
// which should happen when the command being executed has terminated.
func execLogger(outpipe io.Reader, cmdDone <-chan error) {
	cmdScanner := bufio.NewScanner(outpipe)
	//printout:
	for {
		// Check for new line, and write it to execLog
		for cmdScanner.Scan() {
			execLog.Info(cmdScanner.Text())
		}
		// Check for value in cmdDone, or closed channel
		select {
		case err, ok := <-cmdDone:
			if !ok {
				execLog.Warn("Command logger closed before execution completed.")
				return
			}
			if err != nil {
				execLog.Warn("Command execution complete. Error:", err)
			} else {
				execLog.Info("Command execution complete (success).")
			}
			return
			//break printout
		default:
			// Take a break from scanning
			time.Sleep(time.Millisecond * 50)
		}
	}
}

// debugTiming can be called with defer so a function's execution time may be
// logged.  (e.g. defer debugTiming(time.Now(), "someFunction"))
func debugTiming(start time.Time, fun string) {
	log.Debugf("%s completed in %s", fun, time.Since(start))
}

func decodeNetAddr(addr string) dcrutil.Address {
	address, _ := dcrutil.DecodeNetworkAddress(addr)
	return address
}
