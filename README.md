dcrspy
====

**WORK IN PROGRESS**

[![Gitter](https://badges.gitter.im/chappjc/dcrspy.svg)](https://gitter.im/chappjc/dcrspy?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

dcrspy is a program to continuously monitor and log changes in various data
on the Decred network.  It works by connecting to both dcrd and dcrwallet
and responding when a new block is detected via a notifier registered with
dcrd.

## Types of Data

Two types of information are monitored:

* Block chain data (from dcrd)
* Stake information (from your wallet), more wallet info in to-do.

A connection to dcrwallet is optional, but for stake information it is required.

See [Data Details](#data-details) below for details.


## Output

Multiple destinations for the data are planned:

1. **stdout**.  JSON-formatted data is send to stdout. IMPLEMENTED.
2. **File system**.  JSON-formatted data is written to the file system.
   TESTING on branch [`json_file_output`](https://github.com/chappjc/dcrspy/tree/json_file_output).
3. **Database**. Data is inserted into a MySQL database.  NOT IMPLEMENTED.
4. **Plain text summary**: balances, votes, current ticket price, mean fees, wallet status. NEXT.
5. HTTP access via **RESTful API**.  NOT IMPLEMENTED.

## TO-DO

There is a [very long to-do list](https://drive.google.com/open?id=1Z057i7tGfnATWu0w7loetIkGElteNnlx2bJSxPvVOqE).

## Requirements

- [Go](http://golang.org) 1.6 or newer.
- Running `dcrd` synchronized to the current best block on the network.
- (Optional, for stake info) `dcrwallet` connected to `dcrd`.

## Installation

#### Build from Source

- Install Go:
  http://golang.org/doc/install

- Verify Go installation:

`go env GOROOT GOPATH`

- Build executable

`go get -u -v github.com/chappjc/dcrspy`

- Find dcrspy executable in `$GOPATH/bin`, and copy elsewhere (recommended).

## Updating

Run the same command used to build:

`go get -u -v github.com/chappjc/dcrspy`

## Getting Started

By default, dcrspy will monitor both block data and your wallet's stake info, 
and write the data to stdout in JSON format.

There are several program options, which may be utilized via:

1. Command line arguments
2. Config file (e.g. dcrspy.conf)

### Command line

```
Usage:
  dcrspy [OPTIONS]

Application Options:
  -C, --configfile=    Path to configuration file (.../dcrspy.conf)
  -V, --version        Display version information and exit
      --testnet        Use the test network (default mainnet)
      --simnet         Use the simulation test network (default mainnet)
  -d, --debuglevel=    Logging level {trace, debug, info, warn, error, critical} (info)
      --logdir=        Directory to log output (.../logs)
      --noblockdata    Collect block data (default true)
      --nostakeinfo    Collect stake info (default true). Requires wallet connection
      --dcrduser=      Daemon RPC user name
      --dcrdpass=      Daemon RPC password
      --dcrdserv=      Hostname/IP and port of dcrd RPC server to connect to (default localhost:9109, testnet: localhost:19109, simnet:
                       localhost:19556)
      --dcrdcert=      File containing the dcrd certificate file (${HOME}/.dcrd/rpc.cert)
      --dcrwuser=      Wallet RPC user name
      --dcrwpass=      Wallet RPC password
      --dcrwserv=      Hostname/IP and port of dcrwallet RPC server to connect to (default localhost:9110, testnet: localhost:19110,
                       simnet: localhost:19557)
      --dcrwcert=      File containing the dcrwallet certificate file (${HOME}/.dcrwallet/rpc.cert)
      --noclienttls    Disable TLS for the RPC client -- NOTE: This is only allowed if the RPC client is connecting to localhost
      --accountname=   Name of the account from (default: default) (default)
      --ticketaddress= Address to which you have given voting rights
      --pooladdress=   Address to which you have given rights to pool fees

Help Options:
  -h, --help           Show this help message
 ```

### Config file

```ini
[Application Options]

debuglevel=debug

dcrduser=duser
dcrdpass=asdfExample

dcrdserv=localhost:9109
dcrdcert=/home/me/.dcrd/rpc.cert

dcrwuser=wuser
dcrwpass=qwertyExample

dcrwserv=localhost:9110
dcrwcert=/home/me/.dcrwallet/rpc.cert

;noclienttls=false
```

## Data Details

Block chain data obtained from dcrd includes:

1. Block header (hash, voters, height, difficulty, nonce, time, etc.)

 ```
{
    "hash": "0000000000000c21323dc60866aa5a869b57c001589a026cf0a1a18765971d83",
    "confirmations": 1,
    "version": 1,
    "previousblockhash": "000000000000056c3051ae16b4dc098238165e70b11c77b9024c57662dbb479d",
    "merkleroot": "0291270c33212dfeff03da47fa4122ca81825f352bd8cca547c0832e22e67c18",
    "stakeroot": "3112f31e8a03aa588f4755cdd974eaab938c7f221d29322231992a472188a79a",
    "votebits": 1,
    "finalstate": "ca3c601c8042",
    "voters": 5,
    "freshstake": 2,
    "revocations": 1,
    "poolsize": 42630,
    "bits": "1a173a02",
    "sbits": 22.42495867,
    "height": 34625,
    "size": 5945,
    "time": 1465232577,
    "nonce": 4055550757,
    "difficulty": 722316.87132517
}
```

2. Ticket pool info.  This is a custom data structure.

 ```
{
    "poolsize": 42635,
    "poolvalue": 729895.86066478,
    "poolvalavg": 17.11964021
}
```

3. Ticket fee info (block).  This is the usual output of `ticketfeeinfo` with
no extra arguments:

 ```
{
    "height": 34624,
    "number": 3,
    "min": 0.01013513,
    "max": 0.0255102,
    "mean": 0.01558548,
    "median": 0.01111111,
    "stddev": 0.0086089
}
```

4. Current and estimated stake difficulty.  These are the usual outputs of
`estimatestakediff` and `getstakedifficulty`:

 ```
{
    "min": 21.92454703,
    "max": 50.34015709,
    "expected": 39.59322765
}
{
    "current": 22.42495867,
    "next": 22.42495867
}
```

## Issue Tracker

Please report any issues using the [GitHub issue tracker](https://github.com/chappjc/dcrspy/issues).

## License

dcrspy is licensed under the [copyfree](http://copyfree.org) ISC License.

dcrspy borrows its logging and config file facilities, plus some boilerplate
code in main.go, from the dcrticketbuyer project by the Decred developers.
The rest is by Jonathan Chappelow (chappjc).
