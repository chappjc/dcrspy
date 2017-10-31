// DCRSPY is a program to continuously monitor and log changes in various data
// on the Decred network.  It works by connecting to both dcrd and dcrwallet,
// and responding when a new block is detected via a notifier registered with
// dcrd.
//
// Types of information monitored:
//	1. Block chain data (from dcrd)
//  2. Stake information (from your wallet)
//  3. mempool (from dcrd)
//
// See README.md and TODO for more information.
//
// Copyright (c) 2017, Jonathan Chappelow
// See LICENSE for details.
//
// Borrowing logging and config file facilities, plus much boilerplate from
// main.go, from on dcrticketbuyer, by the Decred developers.

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/rpcclient"
)

const (
	spyart = `
                       __                          
                  ____/ /__________________  __  __
                 / __  / ___/ ___/ ___/ __ \/ / / /
                / /_/ / /__/ /  (__  ) /_/ / /_/ / 
                \__,_/\___/_/  /____/ .___/\__, /  
                                   /_/    /____/  
`
)

// mainCore does all the work. Deferred functions do not run after os.Exit(),
// so main wraps this function, which returns a code.
func mainCore() int {
	// Parse the configuration file, and setup logger.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Failed to load dcrspy config: %s\n", err.Error())
		return 1
	}
	defer backendLog.Flush()

	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			log.Critical(err)
			return -1
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Start with version info
	log.Infof(appName+" version %s%v", ver.String(), spyart)

	rpcclient.UseLogger(clientLog)

	log.Debugf("Output folder: %v", cfg.OutFolder)
	log.Debugf("Log folder: %v", cfg.LogDir)

	// Create data output folder if it does not already exist
	if os.MkdirAll(cfg.OutFolder, 0750) != nil {
		fmt.Printf("Failed to create data output folder %s. Error: %s\n",
			cfg.OutFolder, err.Error())
		return 2
	}

	// Connect to dcrd RPC server using websockets. Set up the
	// notification handler to deliver blocks through a channel.
	makeChans(cfg)

	// Daemon client connection
	dcrdClient, nodeVer, err := connectNodeRPC(cfg)
	if err != nil || dcrdClient == nil {
		log.Infof("Connection to dcrd failed: %v", err)
		return 4
	}

	// Display connected network
	curnet, err := dcrdClient.GetCurrentNet()
	if err != nil {
		fmt.Println("Unable to get current network from dcrd:", err.Error())
		return 5
	}
	log.Infof("Connected to dcrd (JSON-RPC API v%s) on %v",
		nodeVer.String(), curnet.String())

	// Validate each watchaddress
	addresses := make([]dcrutil.Address, 0, len(cfg.WatchAddresses))
	addrMap := make(map[string]TxAction)
	var needEmail bool
	if len(cfg.WatchAddresses) > 0 && !cfg.NoMonitor {
		for _, ai := range cfg.WatchAddresses {
			s := strings.Split(ai, ",")

			var emailActn TxAction
			if len(s) > 1 && len(s[1]) > 0 {
				emailI, err := strconv.Atoi(s[1])
				if err != nil {
					log.Error(err)
					continue
				}
				emailActn = TxAction(emailI)
				needEmail = needEmail || (emailActn != 0)
			}

			a := s[0]

			addr, err := dcrutil.DecodeAddress(a)
			// or DecodeNetworkAddress for auto-detection of network
			if err != nil {
				log.Errorf("Invalid watchaddress %v", a)
				return 6
			}
			if _, seen := addrMap[a]; seen {
				continue
			}
			log.Infof("Valid watchaddress: %v", addr)
			addresses = append(addresses, addr)
			addrMap[a] = emailActn
		}
		if len(addresses) == 0 {
			if spyChans.relevantTxMempoolChan != nil {
				close(spyChans.relevantTxMempoolChan)
				spyChans.relevantTxMempoolChan = nil
			}
		}
	}

	emailConfig, err := getEmailConfig(cfg)
	if needEmail && err != nil {
		log.Error("Error parsing email configuration: ", err)
		return 16
	}

	// Register for block connection notifications.
	if err = dcrdClient.NotifyBlocks(); err != nil {
		fmt.Printf("Failed to register daemon RPC client for "+
			"block notifications: %s\n", err.Error())
		return 7
	}

	// Register for stake difficulty change notifications.
	if err = dcrdClient.NotifyStakeDifficulty(); err != nil {
		fmt.Printf("Failed to register daemon RPC client for "+
			"stake difficulty change notifications: %s\n", err.Error())
		return 7
	}

	// Register for tx accepted into mempool ntfns
	if err = dcrdClient.NotifyNewTransactions(false); err != nil {
		fmt.Printf("Failed to register daemon RPC client for "+
			"new transaction (mempool) notifications: %s\n", err.Error())
		return 7
	}

	// For OnNewTickets
	//  Commented since there is a bug in rpcclient/notify.go
	// if err := dcrdClient.NotifyNewTickets(); err != nil {
	// 	fmt.Printf("Failed to register daemon RPC client for  "+
	// 		"new tickets (mempool) notifications: %s\n", err.Error())
	// 	os.Exit(1)
	// }

	if err = dcrdClient.NotifyWinningTickets(); err != nil {
		fmt.Printf("Failed to register daemon RPC client for  "+
			"winning tickets notifications: %s\n", err.Error())
		os.Exit(1)
	}

	// Register a Tx filter for addresses (receiving).  The filter applies to
	// OnRelevantTxAccepted.
	// TODO: register outpoints (third argument).
	if len(addresses) > 0 {
		if err = dcrdClient.LoadTxFilter(true, addresses, nil); err != nil {
			fmt.Printf("Failed to register addresses.  Error: %v", err.Error())
			return 7
		}
	}

	// Wallet

	var dcrwClient *rpcclient.Client
	if !cfg.NoCollectStakeInfo {
		var walletVer semver
		dcrwClient, walletVer, err = connectWalletRPC(cfg)
		if err != nil || dcrwClient == nil {
			log.Infof("Connection to dcrwallet failed: %v", err)
			return 17
		}
		log.Infof("Connected to dcrwallet (JSON-RPC API v%s)",
			walletVer.String())
	}

	// Ctrl-C to shut down.
	// Nothing should be sent the quit channel.  It should only be closed.
	quit := make(chan struct{})
	// Only accept a single CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Start waiting for the interrupt signal
	go func() {
		<-c
		signal.Stop(c)
		// Close the channel so multiple goroutines can get the message
		log.Infof("CTRL+C hit.  Closing goroutines.")
		close(quit)
	}()

	// Saver mutex, to share the same underlying output resource between block
	// and stake info data savers
	saverMutexTerm := new(sync.Mutex)
	saverMutexFiles := new(sync.Mutex)

	// Build a slice of each required saver type for each data source
	var blockDataSavers []BlockDataSaver
	var stakeInfoDataSavers []StakeInfoDataSaver
	var mempoolSavers []MempoolDataSaver
	// JSON to stdout
	if cfg.SaveJSONStdout {
		blockDataSavers = append(blockDataSavers,
			NewBlockDataToJSONStdOut(saverMutexTerm))
		stakeInfoDataSavers = append(stakeInfoDataSavers,
			NewStakeInfoDataToJSONStdOut(saverMutexTerm))
		mempoolSavers = append(mempoolSavers,
			NewMempoolDataToJSONStdOut(saverMutexTerm))
	}
	// JSON to file
	if cfg.SaveJSONFile {
		blockDataSavers = append(blockDataSavers,
			NewBlockDataToJSONFiles(cfg.OutFolder, "block_data-", saverMutexFiles))
		stakeInfoDataSavers = append(stakeInfoDataSavers,
			NewStakeInfoDataToJSONFiles(cfg.OutFolder, "stake-info-", saverMutexFiles))
		mempoolSavers = append(mempoolSavers,
			NewMempoolDataToJSONFiles(cfg.OutFolder, "mempool-info-", saverMutexFiles))
	}

	// If no savers specified, enable Summary Output
	if len(blockDataSavers) == 0 {
		cfg.SummaryOut = true
	}

	summarySaverBlockData := NewBlockDataToSummaryStdOut(saverMutexTerm)
	summarySaverStakeInfo := NewStakeInfoDataToSummaryStdOut(saverMutexTerm)
	summarySaverMempool := NewMempoolDataToSummaryStdOut(cfg.FeeWinRadius, saverMutexTerm)

	if cfg.SummaryOut {
		blockDataSavers = append(blockDataSavers, summarySaverBlockData)
		stakeInfoDataSavers = append(stakeInfoDataSavers, summarySaverStakeInfo)
		mempoolSavers = append(mempoolSavers, summarySaverMempool)
	}

	if cfg.DumpAllMPTix {
		log.Debugf("Dumping all mempool tickets to file in %s.\n", cfg.OutFolder)
		mempoolFeeDumper := NewMempoolFeeDumper(cfg.OutFolder, "mempool-fees",
			saverMutexFiles)
		mempoolSavers = append(mempoolSavers, mempoolFeeDumper)
	}

	// Block data collector
	collector, err := newBlockDataCollector(cfg, dcrdClient)
	if err != nil {
		fmt.Printf("Failed to create block data collector: %s\n", err.Error())
		return 9
	}

	backendLog.Flush()

	// Initial data summary prior to start of regular collection
	blockData, err := collector.collect(!cfg.PoolValue)
	if err != nil {
		fmt.Printf("Block data collection for initial summary failed: %v",
			err.Error())
		return 10
	}

	if err = summarySaverBlockData.Store(blockData); err != nil {
		fmt.Printf("Failed to print initial block data summary: %v",
			err.Error())
		return 11
	}

	// WaitGroup for the monitor goroutines
	var wg sync.WaitGroup

	if !cfg.NoCollectBlockData && !cfg.NoMonitor {
		// Blockchain monitor for the collector
		wg.Add(1)
		// If collector is nil, so is connectChan
		wsChainMonitor := newChainMonitor(collector,
			blockDataSavers, quit, &wg, !cfg.PoolValue,
			addrMap)
		go wsChainMonitor.blockConnectedHandler()
	}

	// Stake info data (getstakeinfo) collector
	var stakeCollector *stakeInfoDataCollector
	if !cfg.NoCollectStakeInfo {
		stakeCollector, err = newStakeInfoDataCollector(cfg, dcrdClient, dcrwClient)
		if err != nil {
			fmt.Printf("Failed to create block data collector: %s\n", err.Error())
			return 12
		}

		// Initial data summary prior to start of regular collection
		height, err := stakeCollector.getHeight()
		if err != nil {
			fmt.Printf("Unable to get current block height. Error: %v", err.Error())
			return 12
		}
		stakeInfoData, err := stakeCollector.collect(height)
		if err != nil {
			fmt.Printf("Stake info data collection failed gathering initial"+
				"data: %v", err.Error())
			return 12
		}

		if err := summarySaverStakeInfo.Store(stakeInfoData); err != nil {
			fmt.Printf("Failed to print initial stake info data summary: %v",
				err.Error())
			return 12
		}

		if !cfg.NoMonitor {
			wg.Add(1)
			// Stake info monitor for the stakeCollector
			wsStakeInfoMonitor := newStakeMonitor(stakeCollector,
				stakeInfoDataSavers, quit, &wg)
			go wsStakeInfoMonitor.blockConnectedHandler()
		}
	}

	if cfg.MonitorMempool {
		mpoolCollector, err := newMempoolDataCollector(cfg, dcrdClient)
		if err != nil {
			fmt.Printf("Failed to create mempool data collector: %s\n", err.Error())
			return 13
		}

		mpData, err := mpoolCollector.collect()
		if err != nil {
			fmt.Printf("Mempool info collection failed while gathering initial"+
				"data: %v", err.Error())
			return 14
		}

		if err := summarySaverMempool.Store(mpData); err != nil {
			fmt.Printf("Failed to print initial mempool info summary: %v",
				err.Error())
			return 15
		}

		newTicketLimit := int32(cfg.MPTriggerTickets)
		mini := time.Duration(cfg.MempoolMinInterval) * time.Second
		maxi := time.Duration(cfg.MempoolMaxInterval) * time.Second

		wg.Add(1)
		mpi := &mempoolInfo{
			currentHeight:               mpData.height,
			numTicketPurchasesInMempool: mpData.numTickets,
			numTicketsSinceStatsReport:  0,
			lastCollectTime:             time.Now(),
		}
		mpm := newMempoolMonitor(mpoolCollector, mempoolSavers,
			quit, &wg, newTicketLimit, mini, maxi, mpi)
		go mpm.txHandler(dcrdClient)

		spyChans.txTicker = time.NewTicker(time.Second * 2)
		go func() {
			for range spyChans.txTicker.C {
				spyChans.newTxChan <- new(chainhash.Hash)
			}
		}()
	}

	// No addresses is implied if NoMonitor is true.
	if len(addresses) > 0 {
		if emailConfig != nil {
			wg.Add(1)
			go EmailQueue(emailConfig, cfg.EmailSubject, &wg, quit)
		}
		wg.Add(1)
		go handleReceivingTx(dcrdClient, addrMap, emailConfig,
			&wg, quit)
		//wg.Add(1)
		//go handleSendingTx(dcrdClient, addrMap, spendTxChan, &wg, quit)
	}

	// stakediff not implemented yet as the notifier appears broken
	go stakeDiffHandler(quit)

	log.Infof("RPC client(s) successfully connected. Now monitoring and " +
		"collecting data.")

	// Wait for CTRL+C to signal goroutines to terminate via quit channel.
	wg.Wait()

	// But if monitoring is disabled, simulate an OS interrupt.
	if cfg.NoMonitor {
		c <- os.Interrupt
		time.Sleep(200)
	}

	// Closing these channels should be unnecessary if quit was handled right
	closeChans()

	if dcrdClient != nil {
		log.Infof("Closing connection to dcrd.")
		dcrdClient.Shutdown()
	}

	if !cfg.NoCollectStakeInfo && dcrwClient != nil {
		log.Infof("Closing connection to dcrwallet.")
		dcrwClient.Shutdown()
	}

	log.Infof("Bye!")
	time.Sleep(500 * time.Millisecond)
	return 16
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

func stakeDiffHandler(quit chan struct{}) {
	for {
		select {
		case s, ok := <-spyChans.stakeDiffChan:
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
}

// debugTiming can be called with defer so a function's execution time may be
// logged.  (e.g. defer debugTiming(time.Now(), "someFunction"))
func debugTiming(start time.Time, fun string) {
	log.Debugf("%s completed in %s", fun, time.Since(start))
}

func decodeNetAddr(addr string) dcrutil.Address {
	address, _ := dcrutil.DecodeAddress(addr)
	return address
}

func getEmailConfig(cfg *config) (emailConf *EmailConfig, err error) {
	smtpHost, smtpPort, err := net.SplitHostPort(cfg.SMTPServer)
	if err != nil {
		return
	}

	smtpPortNum, err := strconv.Atoi(smtpPort)
	if err != nil {
		return
	}

	emailConf = &EmailConfig{
		emailAddr:  cfg.EmailAddr,
		smtpUser:   cfg.SMTPUser,
		smtpPass:   cfg.SMTPPass,
		smtpServer: smtpHost,
		smtpPort:   smtpPortNum,
	}

	return
}

func main() {
	os.Exit(mainCore())
}
