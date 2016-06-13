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
	"github.com/decred/dcrrpcclient"
)

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
				execLog.Info("Full command line to be executed: ", cmd.Path, strings.Join(argsSplit, " "))

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

	log.Infof("Attempting to connect to dcrd RPC %s as user %s "+
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
	log.Debugf("Connected to dcrd on network: %v", net.String())

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

		log.Infof("Attempting to connect to dcrwallet RPC %s as user %s "+
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
	// JSON to stdout
	if cfg.SaveJSONStdout {
		blockDataSavers = append(blockDataSavers, NewBlockDataToJSONStdOut(saverMutex))
		stakeInfoDataSavers = append(stakeInfoDataSavers, NewStakeInfoDataToJSONStdOut(saverMutex))
	}
	// JSON to file
	if cfg.SaveJSONFile {
		blockDataSavers = append(blockDataSavers,
			NewBlockDataToJSONFiles(cfg.OutFolder, "block_data-", saverMutex))
		stakeInfoDataSavers = append(stakeInfoDataSavers,
			NewStakeInfoDataToJSONFiles(cfg.OutFolder, "stake-info-", saverMutex))
	}

	// If no savers specified, enable Summary Output
	if len(blockDataSavers) == 0 {
		cfg.SummaryOut = true
	}

	summarySaverBlockData := NewBlockDataToSummaryStdOut(saverMutex)
	summarySaverStakeInfo := NewStakeInfoDataToSummaryStdOut(saverMutex)

	if cfg.SummaryOut {
		blockDataSavers = append(blockDataSavers, summarySaverBlockData)
		stakeInfoDataSavers = append(stakeInfoDataSavers, summarySaverStakeInfo)
	}

	// Block data collector
	collector, err := newBlockDataCollector(cfg, dcrdClient)
	if err != nil {
		fmt.Printf("Failed to create block data collector: %s\n", err.Error())
		os.Exit(1)
	}

	// Initial data summary prior to start of regular collection
	blockData, err := collector.collect()
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
			blockDataSavers, quit, &wg)
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

	// stakediff not implemented yet as the notifier appears broken
	go func() {
		for {
			select {
			case s, ok := <-stakeDiffChan:
				if !ok {
					log.Infof("Stake difficulty channel closed")
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
