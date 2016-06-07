// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Modified from dcrticketbuyer for dcrspy.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	flags "github.com/btcsuite/go-flags"
	"github.com/decred/dcrutil"
	"github.com/decred/dcrwallet/netparams"
)

const (
	defaultConfigFilename = "dcrspy.conf"
	defaultLogLevel       = "info"
	defaultLogDirname     = "logs"
	defaultLogFilename    = "dcrspy.log"
	currentVersion        = 1
)

var curDir, _ = os.Getwd()
var activeNet = &netparams.MainNetParams

var (
	dcrdHomeDir      = dcrutil.AppDataDir("dcrd", false)
	dcrwalletHomeDir = dcrutil.AppDataDir("dcrwallet", false)
	//dcrspyHomeDir            = dcrutil.AppDataDir("dcrspy", false)
	defaultDaemonRPCKeyFile  = filepath.Join(dcrdHomeDir, "rpc.key")
	defaultDaemonRPCCertFile = filepath.Join(dcrdHomeDir, "rpc.cert")
	defaultConfigFile        = filepath.Join(curDir, defaultConfigFilename)
	defaultWalletRPCKeyFile  = filepath.Join(dcrwalletHomeDir, "rpc.key")
	defaultWalletRPCCertFile = filepath.Join(dcrwalletHomeDir, "rpc.cert")
	defaultLogDir            = filepath.Join(curDir, defaultLogDirname)
	defaultHost              = "localhost"

	defaultAccountName   = "default"
	defaultTicketAddress = ""
	defaultPoolAddress   = ""
)

type config struct {
	// General application behavior
	ConfigFile         string `short:"C" long:"configfile" description:"Path to configuration file"`
	ShowVersion        bool   `short:"V" long:"version" description:"Display version information and exit"`
	TestNet            bool   `long:"testnet" description:"Use the test network (default mainnet)"`
	SimNet             bool   `long:"simnet" description:"Use the simulation test network (default mainnet)"`
	DebugLevel         string `short:"d" long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
	LogDir             string `long:"logdir" description:"Directory to log output"`
	NoCollectBlockData bool   `long:"noblockdata" description:"Collect block data (default true)"`
	NoCollectStakeInfo bool   `long:"nostakeinfo" description:"Collect stake info (default true). Requires wallet connection"`

	// RPC client options
	DcrdUser         string `long:"dcrduser" description:"Daemon RPC user name"`
	DcrdPass         string `long:"dcrdpass" description:"Daemon RPC password"`
	DcrdServ         string `long:"dcrdserv" description:"Hostname/IP and port of dcrd RPC server to connect to (default localhost:9109, testnet: localhost:19109, simnet: localhost:19556)"`
	DcrdCert         string `long:"dcrdcert" description:"File containing the dcrd certificate file"`
	DcrwUser         string `long:"dcrwuser" description:"Wallet RPC user name"`
	DcrwPass         string `long:"dcrwpass" description:"Wallet RPC password"`
	DcrwServ         string `long:"dcrwserv" description:"Hostname/IP and port of dcrwallet RPC server to connect to (default localhost:9110, testnet: localhost:19110, simnet: localhost:19557)"`
	DcrwCert         string `long:"dcrwcert" description:"File containing the dcrwallet certificate file"`
	DisableClientTLS bool   `long:"noclienttls" description:"Disable TLS for the RPC client -- NOTE: This is only allowed if the RPC client is connecting to localhost"`

	// Automatic ticket buyer settings
	AccountName   string `long:"accountname" description:"Name of the account from (default: default)"`
	TicketAddress string `long:"ticketaddress" description:"Address to which you have given voting rights"`
	PoolAddress   string `long:"pooladdress" description:"Address to which you have given rights to pool fees"`
}

var (
	defaultConfig = config{
		DebugLevel:    defaultLogLevel,
		ConfigFile:    defaultConfigFile,
		LogDir:        defaultLogDir,
		DcrdCert:      defaultDaemonRPCCertFile,
		DcrwCert:      defaultWalletRPCCertFile,
		AccountName:   defaultAccountName,
		TicketAddress: defaultTicketAddress,
		PoolAddress:   defaultPoolAddress,
	}
)

// cleanAndExpandPath expands environement variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(dcrwalletHomeDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but they variables can still be expanded via POSIX-style
	// $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		return true
	}
	return false
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsytems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) error {
	// When the specified string doesn't have any delimters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
		// Validate debug log level.
		if !validLogLevel(debugLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, debugLevel)
		}

		// Change the logging level for all subsystems.
		setLogLevels(debugLevel)

		return nil
	}

	// Split the specified string into subsystem/level pairs while detecting
	// issues and update the log levels accordingly.
	for _, logLevelPair := range strings.Split(debugLevel, ",") {
		if !strings.Contains(logLevelPair, "=") {
			str := "The specified debug level contains an invalid " +
				"subsystem/level pair [%v]"
			return fmt.Errorf(str, logLevelPair)
		}

		// Extract the specified subsystem and log level.
		fields := strings.Split(logLevelPair, "=")
		subsysID, logLevel := fields[0], fields[1]

		// Validate subsystem.
		if _, exists := subsystemLoggers[subsysID]; !exists {
			str := "The specified subsystem [%v] is invalid -- " +
				"supported subsytems %v"
			return fmt.Errorf(str, subsysID, supportedSubsystems())
		}

		// Validate log level.
		if !validLogLevel(logLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, logLevel)
		}

		setLogLevel(subsysID, logLevel)
	}

	return nil
}

// loadConfig initializes and parses the config using a config file and command
// line options.
func loadConfig() (*config, error) {
	loadConfigError := func(err error) (*config, error) {
		return nil, err
	}

	// Default config.
	cfg := defaultConfig

	// A config file in the current directory takes precedence.
	if _, err := os.Stat(defaultConfigFilename); !os.IsNotExist(err) {
		cfg.ConfigFile = defaultConfigFile
	}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.
	preCfg := cfg
	preParser := flags.NewParser(&preCfg, flags.Default)
	_, err := preParser.Parse()
	if err != nil {
		e, ok := err.(*flags.Error)
		if !ok || e.Type != flags.ErrHelp {
			preParser.WriteHelp(os.Stderr)
		}
		if e.Type == flags.ErrHelp {
			os.Exit(0)
		}
		return loadConfigError(err)
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", currentVersion)
		os.Exit(0)
	}

	// Load additional config from file.
	var configFileError error
	parser := flags.NewParser(&cfg, flags.Default)
	err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintln(os.Stderr, err)
			parser.WriteHelp(os.Stderr)
			return loadConfigError(err)
		}
		configFileError = err
	}

	// Parse command line options again to ensure they take precedence.
	_, err = parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return loadConfigError(err)
	}

	// Warn about missing config file after the final command line parse
	// succeeds.  This prevents the warning on help messages and invalid
	// options.
	if configFileError != nil {
		log.Warnf("%v", configFileError)
		//fmt.Printf("%v\n",configFileError)
		return loadConfigError(configFileError)
	}

	// Choose the active network params based on the selected network.
	// Multiple networks can't be selected simultaneously.
	numNets := 0
	activeNet = &netparams.MainNetParams
	if cfg.TestNet {
		activeNet = &netparams.TestNetParams
		numNets++
	}
	if cfg.SimNet {
		activeNet = &netparams.SimNetParams
		numNets++
	}
	if numNets > 1 {
		str := "%s: The testnet and simnet params can't be used " +
			"together -- choose one"
		err := fmt.Errorf(str, "loadConfig")
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return loadConfigError(err)
	}

	// Set the host names and ports to the default if the
	// user does not specify them.
	if cfg.DcrdServ == "" {
		cfg.DcrdServ = defaultHost + ":" + activeNet.RPCClientPort
	}
	if cfg.DcrwServ == "" {
		cfg.DcrwServ = defaultHost + ":" + activeNet.RPCServerPort
	}

	// The HTTP server port can not be beyond a uint16's size in value.
	// if cfg.HttpSvrPort > 0xffff {
	// 	str := "%s: Invalid HTTP port number for HTTP server"
	// 	err := fmt.Errorf(str, "loadConfig")
	// 	fmt.Fprintln(os.Stderr, err)
	// 	parser.WriteHelp(os.Stderr)
	// 	return loadConfigError(err)
	// }

	// Append the network type to the log directory so it is "namespaced"
	// per network.
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)
	cfg.LogDir = filepath.Join(cfg.LogDir, activeNet.Name)

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		os.Exit(0)
	}

	// Initialize logging at the default logging level.
	initSeelogLogger(filepath.Join(cfg.LogDir, defaultLogFilename))
	setLogLevels(defaultLogLevel)

	// Parse, validate, and set debug log level(s).
	if err := parseAndSetDebugLevels(cfg.DebugLevel); err != nil {
		err := fmt.Errorf("%s: %v", "loadConfig", err.Error())
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return loadConfigError(err)
	}

	//csvPath = cfg.HttpUIPath

	return &cfg, nil
}
