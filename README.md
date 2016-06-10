# dcrspy

[![Gitter](https://badges.gitter.im/chappjc/dcrspy.svg)](https://gitter.im/chappjc/dcrspy?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

dcrspy is a program to continuously monitor and log changes in various data
on the Decred network.  It works by connecting to both dcrd and dcrwallet
and responding when a new block is detected via a notifier registered with
dcrd.  Communication with dcrd and dcrwallet uses the Decred JSON-RPC API.

## Types of Data

Two types of information are monitored:

* Block chain data (from dcrd)
* Stake and wallet information (from your wallet).

A connection to dcrwallet is optional. Only block data will be obtained when no
wallet connection is available.

See [Data Details](#data-details) below for more information.


## Output

Multiple destinations for the data are planned:

1. **stdout**.  JSON-formatted data is send to stdout. **DONE**.
2. **File system**.  JSON-formatted data is written to the file system. **DONE**.
3. **Database**. Data is inserted into a MySQL database. NOT IMPLEMENTED.
4. **Plain text summary**: balances, votes, current ticket price, mean fees, 
   wallet status. **DONE**.
5. **RESTful API** over HTTPS. NOT IMPLEMENTED.

Details of the JSON output may be found in [Data Details](#data-details).  The
plain text summary looks something like the following (_wallet data redacted_):

~~~none
Block 35561:
        Stake difficulty:                    22.663 -> 22.663 (current -> next block)
        Estimated price in next window:      25.279 / [24.63, 26.68] ([min, max])
        Window progress:   138 / 144  of price window number 246
        Ticket fees:  0.0101, 0.0101, 0.0000 (mean, median, std), n=1
        Ticket pool:  42048 (size), 17.721 (avg. price), 745115.63 (total DCR locked)

Wallet and Stake Info at Height 35561:
- Balances
        Balances (spendable):     0.0000 (default),    0.0000 (all)
        Balances (locked):      xxx.xxxx (default), xxxx.xxxx (all), xxxx.xxxx (imported)
        Balances (any):        xxxx.xxxx (default), xxxx.xxxx (all)
- Stake Info
        ===>  Mining enabled: true;  Unlocked: true  <===
        Mined tickets:        4 (immature),     43 (live)
        mempool tickets:      0 (own),            6 (all)
        Ticket price:      22.663  |    Window progress: 138 / 144
        Wallet's price:     23.8100;  fee:   0.1940 / KiB
        Totals:        541  votes,     919.84 subsidy
                         1 missed,          1 revoked
~~~

## TO-DO

dcrspy is functional, but also a **work-in-progress**.  However, I will try to keep
`master` as stable as possible, and develop new features in separate branches.

There is a [very long to-do list](https://drive.google.com/open?id=1Z057i7tGfnATWu0w7loetIkGElteNnlx2bJSxPvVOqE).

## Requirements

* [Go](http://golang.org) 1.6 or newer.
* Running `dcrd` synchronized to the current best block on the network.
* (Optional, for stake info) `dcrwallet` connected to `dcrd`.

## Installation

### Build from Source

* Install Go:
  http://golang.org/doc/install

* Verify Go installation:

 `go env GOROOT GOPATH`

* Build executable

 `go get -u -v github.com/chappjc/dcrspy`

* Find dcrspy executable in `$GOPATH/bin`, and copy elsewhere (recommended).

## Updating

Run the same command used to build:

`go get -u -v github.com/chappjc/dcrspy`

## Getting Started

By default, dcrspy will monitor both block data and your wallet, and write a
plain text summary of the data to stdout for each new block that is detected.

There are several program options, which may be utilized via:

1. Command line arguments
1. Config file (e.g. dcrspy.conf)

### Command line

Quick tips:

* Get a quick summary and exit, with `-e, --nomonitor`.
* Stay connected and monitor for new blocks, writting:
  * Plain text summary to stdout, with `-s, --summary`. 
  * JSON to stdout, with `-o, --save-jsonstdout`.
  * JSON to file system, with `-j, --save-jsonfile`.
* To monitor only block data (no wallet connection), use `--nostakeinfo`.

The full list of command line switches is below.

~~~ none
Usage:
  dcrspy.exe [OPTIONS]

Application Options:
  /C, /configfile:       Path to configuration file
                         (...\dcrspy.conf)
  /V, /version           Display version information and exit
      /testnet           Use the test network (default mainnet)
      /simnet            Use the simulation test network (default mainnet)
  /d, /debuglevel:       Logging level {trace, debug, info, warn, error,
                         critical} (info)
  /q, /quiet             Easy way to set debuglevel to error
      /logdir:           Directory to log output (...\logs)
  /e, /nomonitor         Do not launch monitors. Display current data and
                         (e)xit.
      /noblockdata       Do not collect block data (default false)
      /nostakeinfo       Do not collect stake info data (default false)
  /f, /outfolder:        Folder for file outputs (...\spydata)
  /s, /summary           Write plain text summary of key data to stdout
  /o, /save-jsonstdout   Save JSON-formatted data to stdout
  /j, /save-jsonfile     Save JSON-formatted data to file
      /dcrduser:         Daemon RPC user name
      /dcrdpass:         Daemon RPC password
      /dcrdserv:         Hostname/IP and port of dcrd RPC server to connect to
                         (default localhost:9109, testnet: localhost:19109,
                         simnet: localhost:19556)
      /dcrdcert:         File containing the dcrd certificate file
                         (%localappdata%\Dcrd\rpc.cert)
      /dcrwuser:         Wallet RPC user name
      /dcrwpass:         Wallet RPC password
      /dcrwserv:         Hostname/IP and port of dcrwallet RPC server to
                         connect to (default localhost:9110, testnet:
                         localhost:19110, simnet: localhost:19557)
      /dcrwcert:         File containing the dcrwallet certificate file
                         (%localappdata%\Dcrwallet\rpc.cert)
      /noclienttls       Disable TLS for the RPC client -- NOTE: This is only
                         allowed if the RPC client is connecting to localhost
      /accountname:      Name of the account from (default: default) (default)
      /ticketaddress:    Address to which you have given voting rights
      /pooladdress:      Address to which you have given rights to pool fees

Help Options:
  /?                     Show this help message
  /h, /help              Show this help message
~~~

### Config file

All command line switches may be placed into the config file, which is
dcrspy.conf by default.

~~~ini
[Application Options]

debuglevel=debug

; Default outfolder is a folder called "dcrspy" in the working directory.
; Change this with the outfolder option:
; Windows
outfolder=%appdata%/dcrspy/spydata
; Linux
; outfolder=$HOME/dcrspy/spydata

; Uncomment for testnet
;testnet=1

dcrduser=duser
dcrdpass=asdfExample

dcrdserv=localhost:9109
dcrdcert=/home/me/.dcrd/rpc.cert

dcrwuser=wuser
dcrwpass=qwertyExample

dcrwserv=localhost:9110
dcrwcert=/home/me/.dcrwallet/rpc.cert
~~~

## Data Details

Block chain data obtained from dcrd includes several types of data.  The JSON
file written by dcrspy for block data is named `block-data-[BLOCKNUM].json`. It
contains a single JSON object, with each data type as a tagged JSON child
object.

1. Block header (hash, voters, height, difficulty, nonce, time, etc.)

 ~~~json
"block_header": {
	"hash": "00000000000014c19867d5cd0f60d9409cd9e4ea68f656dac50befa756866cf8",
	"confirmations": 1,
	"version": 1,
	"previousblockhash": "00000000000010c295f2e808af78d8240c3365d9d52b28e2061f9a55ce9dcd29",
	"merkleroot": "731c342c75237fe72e2d27a6820f6384498add97f96dcef9c6f2fd558a80f4c9",
	"stakeroot": "c3d75f22a1e9bb4b50155f95d9b7a977f7ed1ecaf05024e157a77d7b5697fe04",
	"votebits": 1,
	"finalstate": "6acc9d2694f3",
	"voters": 5,
	"freshstake": 3,
	"revocations": 0,
	"poolsize": 42084,
	"bits": "1a166536",
	"sbits": 22.66271576,
	"height": 35552,
	"size": 3764,
	"time": 1465510687,
	"nonce": 2821830658,
	"difficulty": 749126.76453394
}
~~~

1. Ticket pool info.  This is a custom data structure.

 ~~~json
"ticket_pool_info": {
	"poolsize": 42084,
	"poolvalue": 745705.38747115,
	"poolvalavg": 17.71945127
}
~~~

1. Ticket fee info (block).  This is the usual output of `ticketfeeinfo` with
no extra arguments:

 ~~~json
"ticketfeeinfo_block": {
	"height": 35552,
	"number": 3,
	"min": 0.01010101,
	"max": 0.01013513,
	"mean": 0.01011238,
	"median": 0.01010101,
	"stddev": 1.97e-05
}
~~~

1. Current and estimated stake difficulty.  These are the usual outputs of
`estimatestakediff` and `getstakedifficulty`:

 ~~~json
"estimatestakediff": {
	"min": 23.80700879,
	"max": 28.94194858,
	"expected": 25.46730598
},
"currentstakediff": {
	"current": 22.66271576,
	"next": 22.66271576
}
~~~

Wallet data is stored in a similar manner in file `stake-info-[BLOCKNUM].json`.
There are three data types, tagged `"getstakeinfo`", `"walletinfo"`, and
`"balances"`.  TODO: Update this README with a testnet example output.

## Issue Tracker

Please report any issues using the [GitHub issue tracker](https://github.com/chappjc/dcrspy/issues).

## License

dcrspy is licensed under the [copyfree](http://copyfree.org) ISC License.

dcrspy borrows its logging and config file facilities, plus some boilerplate
code in main.go, from the dcrticketbuyer project by the Decred developers.
The rest is by chappjc.
