# dcrspy

<!--- #[![release version](https://badge.fury.io/gh/chappjc%2Fdcrspy.svg)](https://badge.fury.io/gh/chappjc%2Fdcrspy) -->
[![Build Status](http://img.shields.io/travis/chappjc/dcrspy.svg)](https://travis-ci.org/chappjc/dcrspy)
[![GitHub release](https://img.shields.io/github/release/chappjc/dcrspy.svg)](https://github.com/chappjc/dcrspy/releases)
[![Latest tag](https://img.shields.io/github/tag/chappjc/dcrspy.svg)](https://github.com/chappjc/dcrspy/tags)
[![AUR](https://img.shields.io/aur/version/dcrspy.svg)](https://aur.archlinux.org/packages/dcrspy/)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

dcrspy is a program to continuously monitor and log changes in various data on
the Decred network.  It works by connecting to both dcrd and dcrwallet and
responding to varioius event detected on the network via [notifiers registered
with dcrd over a websocket][1].  Communication with dcrd and dcrwallet uses the
[Decred JSON-RPC API][2].

## Compatibility Notices

* After Decred (i.e. dcrwallet and dcrd) v0.6.0, the notifications API was
changed, requiring dcrspy to update it's notification handlers.  Practically,
this means that for version 0.6.0 and earlier of Decred it is required to use
the [compatibility release
"Millbarge"](https://github.com/chappjc/dcrspy/releases/tag/v0.6.0) or the
[old-ntfns branch](https://github.com/chappjc/dcrspy/tree/old-ntfns), and for
any Decred release *after* 0.6.0 use *at least* dcrspy v0.7.0, preferably
[latest](https://github.com/chappjc/dcrspy/releases), or master. The version of
dcrspy on master will use the new `version` RPC to check that the RPC server has
a compatible API version.
* After Decred v0.7.0, the getbalance RPC response was changed. For Decred
release 0.8.0 and builds of using dcrd commit
[`f5c0b7e`](https://github.com/decred/dcrd/commit/f5c0b7eff2f9336a01a31a344a0bdb1572403e06)
and later, it is necessary to use at least v0.8.0 of dcrspy.

## Types of Data

The types of information monitored are:

* Block chain (from dcrd)
* Stake and wallet (from your wallet, optional).
* Mempool, including ticket fees and transactions of interest (from dcrd)

A connection to dcrwallet is optional, but required for stake info and balances.
See [Data Details](#data-details) below for more information.

Transactions sending to **watched addresses** may be reported (using the
`watchaddress` flag).

## Output

Multiple destinations for the data are planned:

1. **Plain text summary**: balances, votes, current ticket price, mean fees,
   wallet status. **DONE**.
1. **JSON (stdout)**.  JSON-formatted data is send to stdout. **DONE**.
1. **File system**.  JSON-formatted data is written to the file system. **DONE**.
1. **email**: email notification upon receiving to a watched address. **DONE**.
1. **Database**. Data is inserted into a MySQL or MongoDB database.
1. **RESTful API**.

Details of the JSON output may be found in [Data Details](#data-details).  The
plain text summary looks something like the following (_wallet data simulated_):

~~~none
Block 105001:
  Stake difficulty:                   119.385 -> 119.385 (current -> next block)
  Estimated price in next window:      79.021 / [79.02, 315.71] ([min, max])
  Window progress:    26 / 144 in price window number 729
  Ticket fees:  0.0000, 0.0000, 0.0000 (mean, median, std), n=0
  Node connections:  8

Wallet and Stake Info at Height 105001:
- Balances (by account)
  imported:   1234.5678 (any),    0.0000 (spendable), 1234.5678 (locked),    0.0000 (immat.)
  default:    4321.4321 (any), 3210.4000 (spendable),    0.0000 (locked), 1111.0321 (immat.)

- Balances (by type)
  spendable:        3210.4000 (default),  3210.4000 (all)
  locked:              0.0000 (default),  1234.5678 (all), 1234.5678 (imported)
  immat. coinbase:     0.0000 (default),     0.0000 (all)
  immat. votes:     1111.0321 (default),  1111.0321 (all)
  any:              4321.4321 (default),  5555.9999 (all)

- Stake Info:
  Mined tickets:     12 (immature),   57 (live)
  mempool tickets:    5 (own),       544 (everyone)

      ===>  Mining enabled: false;  Unlocked: false  <===
  Ticket price:  119.385   |  Window progress:  26 / 144
  Your limit:   1337.00000;    Fee:   0.0470 DCR / KB
     (Approximately 26.7 tickets may be purchased with set fee.)

  Totals:      404  votes,    697.47 subsidy
                 3 missed,         3 revoked (2 expired)
~~~

Note: Ticket pool value takes up to 10 seconds to compute, so by default it is
not requested from dcrd, and thus not shown in the summary.  It is still
present in JSON, but the values are {0, -1, -1}.  To get actualy ticket pool
value, use `-p, --poolvalue`.

## Watched Addresses and Email Notifications

dcrspy may watch for transactions receiving into or sending from "watched"
addresses.  Watched addresses are specified with the `watchaddress` flag, with
multiple addresses specified using repeated `watchaddress` flags (e.g. one per
line in the config file).  For example:

~~~none
; Addresses to watch for incoming transactions
; Decred developer (C0) address
;watchaddress=Dcur2mcGjmENx4DhNqDctW5wJCVyT3Qeqkx
; Some larger mining pool addresses:
;watchaddress=DsYAN3vT15rjzgoGgEEscoUpPCRtwQKL7dQ
;watchaddress=DshZYJySTD4epCyoKRjPMyVmSvBpFuNYuZ4
~~~

To receive an email notification for each transaction receiving to a watched
address, concatenate at the end of the address: "`,1`" for mined transactions,
"`,2`" for transactions inserted into mempool, or "`,3`" for both.  For example:

~~~none
; Receive email notifications for this one
;watchaddress=DsZWrNNyKDUFPNMcjNYD7A8k9a4HCM5xgsW,1
; But not this one
;watchaddress=Dsg2bQy2yt2onEcaQhT1X9UbTKNtqmHyMus,0
; and not by default
;watchaddress=DskFbReCFNUjVHDf2WQP7AUKdB27EfSPYYE
; Mined and mempool for this:
;watchaddress=DsWM9WbE3YyRKxGGNdBYXCwJoxiBtXFrZBL,3
~~~

An SMTP server name, port, authentication information, and a recipient email
address must also be specified to use email notifications.

~~~none
emailaddr=chappjc@receiving.com
emailsubj="dcrspy tx notification"
smtpuser=smtpuser@mailprovider.net
smtppass=suPErSCRTpasswurd
smtpserver=smtp.mailprovider.org:587
~~~

If you have trouble getting it to work, try any alternate ports available on
your SMTP server (e.g. 587 instead of 465).  You must specify the port.

## Arbitrary Command Execution

When dcrspy receives a new block notification from dcrd, data collection and
recording is triggered. In addition, any system command may be executed in
response to the new block.  The flags to specify the command and the arguments
to be used are:

    -c, --cmdname=         Command name to run. Must be on %PATH%.
    -a, --cmdargs=         Comma-separated list of arguments for command to run.
                           The specifier %n is substituted for block number at
                           execution, and %h is substituted for block hash.

The command name must be an executable (binary or script) on your shell's PATH,
which is `$PATH` in *NIX, and `%PATH%` in Windows.

TODO: Delay execution, or run after data saving is complete.

### Command Arguments

Any arguments must be specified with
cmdargs as a comma-separated list of strings.  For example:

    -c ping -a "127.0.0.1,-n,8"

will execute the following on Linux:

    /usr/bin/ping 127.0.0.1 -n 8

Specifying multiple arguments without commas, using spaces directly, is
incorrect. For example, `-a "127.0.0.1 -n 8"` will not work.

Note that if your command line arguments need to start with a dash (`-`) it is
necessary to use the config file.  For example,

    cmdname=ls
    cmdargs="-al"

### Block Hash and Height Substitution

The new block hash and height at the time of command execution may be included
on the command line using %h and %n, which are substituted for block hash and
number. For example,

    cmdname=echo
    cmdargs="New best block hash: %h; height: %n"

results in the following log entries (date removed for brevity):

    [INF] DCRD: Block height 36435 connected
    [INF] EXEC: New best block hash: 000000000000070f7a0593aee0728d6b3334c1e454da06efc0138008dc1b1cbd; height: 36435
    [INF] EXEC: Command execution complete (success).

Note that the above command used a semicolon since a comma would have indicated
a second argument and been replaced with a space by echo.

### Command Logging

User-specified system command execution uses the logging subsystem tagged with
EXEC in the logs. Both stdout and stderr for the executed command are sent to
the dcrspy log.  The end of command execution is also logged, as shown in the
example above.

## TO-DO

dcrspy is functional, but also a **work-in-progress**.  However, I will try to keep
`master` as stable as possible, and develop new features in separate branches.

## Requirements

* [Go](http://golang.org) 1.7 or newer.
* Running `dcrd` synchronized to the current best block on the network.
* (Optional, for stake info) `dcrwallet` connected to `dcrd`.

Verify that the versions according to the [compatibility notices](#compatibility-notices) above.

## Installation

### Build from Source

The following instructions assume a Unix-like shell (e.g. bash).

* [Install Go](http://golang.org/doc/install)

* Verify Go installation:

        go env GOROOT GOPATH

* Ensure $GOPATH/bin is on your $PATH
* Install glide

        go get -u -v github.com/Masterminds/glide

* Clone dcrspy repo

        git clone https://github.com/chappjc/dcrspy.git $GOPATH/src/github.com/chappjc/dcrspy

* Glide install, and build executable

        cd $GOPATH/src/github.com/chappjc/dcrspy
        glide install
        go install $(glide nv)

* Find dcrspy executable in `$GOPATH/bin`, and copy elsewhere (recommended).

If you receive build errors, it may be due to "vendor" directories left by
glide builds of dependencies such as dcrwallet.  You may safely delete vendor
folders.

## Updating

First, update the repository (assuming you have `master` checked out):

    cd $GOPATH/src/github.com/chappjc/dcrspy
    git pull

Then follow the install instructions starting at "Glide install...".

## Getting Started

By default, dcrspy will monitor both block data and your wallet, and write a
plain text summary of the data to stdout for each new block that is detected.

There are several program options, which may be utilized via:

1. Command line arguments
1. Config file (e.g. dcrspy.conf)

### Command line

Quick tips:

* Get a quick summary and exit, with `-e, --nomonitor`.
* Enable mempool info with `-m, --mempool`.
* Dump all mempool ticket fees to file with `--dumpallmptix`.
* Stay connected and monitor for new blocks, writting:
  * Plain text summary to stdout, with `-s, --summary` (default.)
  * JSON to stdout, with `-o, --save-jsonstdout`.
  * JSON to file system, with `-j, --save-jsonfile`.
* To monitor only block data (no wallet connection), use `--nostakeinfo`.

The full list of command line switches is below, with current directory
replaced by `...`:

~~~ none
$ ./dcrspy -h
Usage:
  dcrspy [OPTIONS]

Application Options:
  -C, --configfile=        Path to configuration file (./dcrspy.conf)
  -V, --version            Display version information and exit
      --testnet            Use the test network (default mainnet)
      --simnet             Use the simulation test network (default mainnet)
  -d, --debuglevel=        Logging level {trace, debug, info, warn, error, critical} (info)
  -q, --quiet              Easy way to set debuglevel to error
      --logdir=            Directory to log output (./logs)
      --cpuprofile=        File for CPU profiling.
  -c, --cmdname=           Command name to run. Must be on %PATH%.
  -a, --cmdargs=           Comma-separated list of arguments for command to run. The specifier %n
                           is substituted for block height at execution, and %h is substituted for
                           block hash.
  -e, --nomonitor          Do not launch monitors. Display current data and (e)xit.
  -m, --mempool            Monitor mempool for new transactions, and report ticketfee info when new
                           tickets are added.
      --mp-min-interval=   The minimum time in seconds between mempool reports, regarless of number
                           of new tickets seen. (4)
      --mp-max-interval=   The maximum time in seconds between mempool reports (within a couple
                           seconds), regarless of number of new tickets seen. (120)
      --mp-ticket-trigger= The number minimum number of new tickets that must be seen to trigger a
                           new mempool report. (4)
  -r, --feewinradius=      Half-width of a window around the ticket with the lowest mineable fee.
      --dumpallmptix       Dump to file the fees of all the tickets in mempool.
      --noblockdata        Do not collect block data (default false)
      --nostakeinfo        Do not collect stake info data (default false)
  -p, --poolvalue          Collect ticket pool value information (8-9 sec).
  -w, --watchaddress=      Watched address (receiving). One per line.
      --smtpuser=          SMTP user name
      --smtppass=          SMTP password
      --smtpserver=        SMTP host name
      --emailaddr=         Destination email address for alerts
      --emailsubj=         Email subject. (default "dcrspy transaction notification") (dcrspy
                           transaction notification)
  -s, --summary            Write plain text summary of key data to stdout
  -o, --save-jsonstdout    Save JSON-formatted data to stdout
  -j, --save-jsonfile      Save JSON-formatted data to file
  -f, --outfolder=         Folder for file outputs (./spydata)
      --dcrduser=          Daemon RPC user name
      --dcrdpass=          Daemon RPC password
      --dcrdserv=          Hostname/IP and port of dcrd RPC server to connect to (default
                           localhost:9109, testnet: localhost:19109, simnet: localhost:19556)
      --dcrdcert=          File containing the dcrd certificate file (~/.dcrd/rpc.cert)
      --nodaemontls        Disable TLS for the daemon RPC client -- NOTE: This is only allowed if
                           the RPC client is connecting to localhost
      --dcrwuser=          Wallet RPC user name
      --dcrwpass=          Wallet RPC password
      --dcrwserv=          Hostname/IP and port of dcrwallet RPC server to connect to (default
                           localhost:9110, testnet: localhost:19110, simnet: localhost:19557)
      --dcrwcert=          File containing the dcrwallet certificate file
                           (~/.dcrwallet/rpc.cert)
      --nowallettls        Disable TLS for the wallet RPC client -- NOTE: This is only allowed if
                           the RPC client is connecting to localhost

Help Options:
  -h, --help               Show this help message
~~~

### Config file

All command line switches may be placed into the config file, which is
dcrspy.conf by default.

~~~ini
[Application Options]

debuglevel=debug
;debuglevel=DSPY=info,EXEC=debug,MEMP=debug,DCRD=info,DCRW=info,RPCC=info

;cmdname=echo
;cmdargs="New best block hash: %h; height: %n"
;cmdname=ping
;cmdargs="127.0.0.1,-n,8"

; Monitor mempool for new tickets, displaying fees
mempool=true
;mp-min-interval=4
;mp-max-interval=120
;mp-ticket-trigger=4
; Show the ticket fee at the limit of mineability (20th or highest), plus show
; windows of fees above and below this limit within the specified radius.
;feewinradius=20
; Dump the prices of all tickets in mempool to disk each time it is reported
; to stdout according to the settings above. JSON formatted output.
;dumpallmptix=1

; Addresses to watch for incoming transactions
; Decred developer (C0) address
;watchaddress=Dcur2mcGjmENx4DhNqDctW5wJCVyT3Qeqkx
; Some larger mining pool addresses:
;watchaddress=DsYAN3vT15rjzgoGgEEscoUpPCRtwQKL7dQ
;watchaddress=DshZYJySTD4epCyoKRjPMyVmSvBpFuNYuZ4
; receive email notifications for this one
;watchaddress=DsZWrNNyKDUFPNMcjNYD7A8k9a4HCM5xgsW,1
; but not this one
;watchaddress=Dsg2bQy2yt2onEcaQhT1X9UbTKNtqmHyMus,0
; and not by default
;watchaddress=DskFbReCFNUjVHDf2WQP7AUKdB27EfSPYYE

; SMTP server setup
;emailaddr=you@receiving.com
;emailsubj="dcrspy tx notification"
;smtpuser=smtpuser@mailprovider.net
;smtppass=suPErSCRTpasswurd
;smtpserver=smtp.mailprovider.org:587

; Ticket pool value takes a long time, 8-9 sec, so the default is false.
;poolvalue=false

; Default outfolder is a folder called "dcrspy" in the working directory.
; Change this with the outfolder option:
; Windows
; outfolder=%appdata%/dcrspy/spydata
; Linux
; outfolder=$HOME/dcrspy/spydata

dcrduser=duser
dcrdpass=asdfExample

dcrdserv=localhost:9109
dcrdcert=/home/me/.dcrd/rpc.cert
;nodaemontls=1

dcrwuser=wuser
dcrwpass=qwertyExample

dcrwserv=localhost:9110
dcrwcert=/home/me/.dcrwallet/rpc.cert
;nowallettls=1

;noclienttls=false
~~~

## Data Details

Block chain data obtained from dcrd includes several types of data.  Most of the
information is written to stdout in a plaintext summary format. Optionally, the
data may be written to disk in JSON format.  The JSON file written by dcrspy for
block data is named `block-data-[BLOCKNUM].json`. It contains a single JSON
object, with each data type as a tagged JSON child object.

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

## Issues

Please report any issues using the [GitHub issue tracker](https://github.com/chappjc/dcrspy/issues).

You may also visit the [dcrspy thread](https://forum.decred.org/threads/decred-monitor-and-data-collector-dcrspy.3838/) at the official Decred forum.

## License

dcrspy is licensed under the [copyfree](http://copyfree.org) ISC License.

dcrspy borrows its logging and config file facilities, plus some boilerplate
code in main.go, from the dcrticketbuyer project by the Decred developers.
The rest is by chappjc.

[1]: https://godoc.org/github.com/decred/dcrd/rpcclient
[2]: https://github.com/decred/dcrd/blob/master/docs/json_rpc_api.md
