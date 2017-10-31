// Interface for saving/storing blockData and stakeInfoData.
// Create a BlockDataSaver or StakeInfoDataSaver by implementing the
// Store(*blockData) or Store(*stakeInfoData) methods.
//
// chappjc

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type fileSaver struct {
	folder   string
	nameBase string
	file     os.File
	mtx      *sync.Mutex
}

// BlockDataSaver is an interface for saving/storing blockData
type BlockDataSaver interface {
	Store(data *blockData) error
}

// BlockDataToJSONStdOut implements BlockDataSaver interface for JSON output to
// stdout
type BlockDataToJSONStdOut struct {
	mtx *sync.Mutex
}

// BlockDataToSummaryStdOut implements BlockDataSaver interface for plain text
// summary to stdout
type BlockDataToSummaryStdOut struct {
	mtx *sync.Mutex
}

// BlockDataToJSONFiles implements BlockDataSaver interface for JSON output to
// the file system
type BlockDataToJSONFiles struct {
	fileSaver
}

// BlockDataToMySQL implements BlockDataSaver interface for output to a
// MySQL database
// type BlockDataToMySQL struct {
// 	mtx *sync.Mutex
// }

// NewBlockDataToJSONStdOut creates a new BlockDataToJSONStdOut with optional
// existing mutex
func NewBlockDataToJSONStdOut(m ...*sync.Mutex) *BlockDataToJSONStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &BlockDataToJSONStdOut{m[0]}
	}
	return &BlockDataToJSONStdOut{}
}

// NewBlockDataToSummaryStdOut creates a new BlockDataToSummaryStdOut with
// optional existing mutex
func NewBlockDataToSummaryStdOut(m ...*sync.Mutex) *BlockDataToSummaryStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &BlockDataToSummaryStdOut{m[0]}
	}
	return &BlockDataToSummaryStdOut{}
}

// NewBlockDataToJSONFiles creates a new BlockDataToJSONFiles with optional
// existing mutex
func NewBlockDataToJSONFiles(folder string, fileBase string,
	m ...*sync.Mutex) *BlockDataToJSONFiles {
	if len(m) > 1 {
		panic("Too many inputs.")
	}

	var mtx *sync.Mutex
	if len(m) > 0 {
		mtx = m[0]
	} else {
		mtx = new(sync.Mutex)
	}

	return &BlockDataToJSONFiles{
		fileSaver: fileSaver{
			folder:   folder,
			nameBase: fileBase,
			file:     os.File{},
			mtx:      mtx,
		},
	}
}

// Store writes blockData to stdout in JSON format
func (s *BlockDataToJSONStdOut) Store(data *blockData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the block data results in to a single JSON object, indented
	jsonConcat, err := JSONFormatBlockData(data)
	if err != nil {
		return err
	}

	// Write JSON to stdout with guards to delimit the object from other text
	fmt.Printf("\n--- BEGIN blockData JSON ---\n")
	_, err = writeFormattedJSONBlockData(jsonConcat, os.Stdout)
	fmt.Printf("--- END blockData JSON ---\n\n")

	return err
}

// Store writes blockData to stdout as plain text summary
func (s *BlockDataToSummaryStdOut) Store(data *blockData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	winSize := activeNet.StakeDiffWindowSize

	fmt.Printf("\nBlock %v:\n", data.header.Height)

	fmt.Printf("  Stake difficulty:                 %9.3f -> %.3f (current -> next block)\n",
		data.currentstakediff.CurrentStakeDifficulty,
		data.currentstakediff.NextStakeDifficulty)

	fmt.Printf("  Estimated price in next window:   %9.3f / [%.2f, %.2f] ([min, max])\n",
		data.eststakediff.Expected, data.eststakediff.Min, data.eststakediff.Max)
	fmt.Printf("  Window progress:   %3d / %3d in price window number %v\n",
		data.idxBlockInWindow, winSize, data.priceWindowNum)

	fmt.Printf("  Ticket fees:  %.4f, %.4f, %.4f (mean, median, std), n=%d\n",
		data.feeinfo.Mean, data.feeinfo.Median, data.feeinfo.StdDev,
		data.feeinfo.Number)

	if data.poolinfo.PoolValue >= 0 {
		fmt.Printf("  Ticket pool:  %v (size), %.3f (avg. price), %.2f (total DCR locked)\n",
			data.poolinfo.PoolSize, data.poolinfo.PoolValAvg, data.poolinfo.PoolValue)
	}

	fmt.Printf("  Node connections:  %d\n", data.connections)

	return nil
}

// Store writes blockData to a file in JSON format
// The file name is nameBase+height+".json".
func (s *BlockDataToJSONFiles) Store(data *blockData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the block data results in to a single JSON object, indented
	jsonConcat, err := JSONFormatBlockData(data)
	if err != nil {
		return err
	}

	// Write JSON to a file with block height in the name
	height := data.header.Height
	fname := fmt.Sprintf("%s%d.json", s.nameBase, height)
	fullfile := filepath.Join(s.folder, fname)
	fp, err := os.Create(fullfile)
	if err != nil {
		log.Errorf("Unable to open file %v for writing.", fullfile)
		return err
	}
	defer fp.Close()

	s.file = *fp
	_, err = writeFormattedJSONBlockData(jsonConcat, &s.file)

	return err
}

func writeFormattedJSONBlockData(jsonConcat *bytes.Buffer, w io.Writer) (int, error) {
	n, err := fmt.Fprintln(w, jsonConcat.String())
	// there was once more, perhaps again.
	return n, err
}

// JSONFormatBlockData concatenates block data results into a single JSON
// object with primary keys for the result type
func JSONFormatBlockData(data *blockData) (*bytes.Buffer, error) {
	var jsonAll bytes.Buffer

	jsonAll.WriteString("{\"estimatestakediff\": ")
	stakeDiffEstJSON, err := json.Marshal(data.eststakediff)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(stakeDiffEstJSON)
	//stakeDiffEstJSON, err := json.MarshalIndent(data.eststakediff, "", "    ")
	//fmt.Println(string(stakeDiffEstJSON))

	jsonAll.WriteString(",\"currentstakediff\": ")
	stakeDiffJSON, err := json.Marshal(data.currentstakediff)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(stakeDiffJSON)

	jsonAll.WriteString(",\"ticketfeeinfo_block\": ")
	feeInfoJSON, err := json.Marshal(data.feeinfo)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(feeInfoJSON)

	jsonAll.WriteString(",\"block_header\": ")
	blockHeaderJSON, err := json.Marshal(data.header)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(blockHeaderJSON)

	jsonAll.WriteString(",\"ticket_pool_info\": ")
	poolInfoJSON, err := json.Marshal(data.poolinfo)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(poolInfoJSON)

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	err = json.Indent(&jsonAllIndented, jsonAll.Bytes(), "", "    ")
	if err != nil {
		return nil, err
	}

	return &jsonAllIndented, err
}

// StakeInfoDataSaver is an interface for saving/storing stakeInfoData
type StakeInfoDataSaver interface {
	Store(data *stakeInfoData) error
}

// StakeInfoDataToJSONStdOut implements StakeInfoDataSaver interface for JSON
// output to stdout
type StakeInfoDataToJSONStdOut struct {
	mtx *sync.Mutex
}

// StakeInfoDataToSummaryStdOut implements StakeInfoDataSaver interface for
// plain text summary to stdout
type StakeInfoDataToSummaryStdOut struct {
	mtx *sync.Mutex
}

// StakeInfoDataToJSONFiles implements StakeInfoDataSaver interface for JSON
// output to the file system
type StakeInfoDataToJSONFiles struct {
	fileSaver
}

// StakeInfoDataToMySQL implements StakeInfoDataSaver interface for output to a
// MySQL database
// type StakeInfoDataToMySQL struct {
// 	mtx *sync.Mutex
// }

// NewStakeInfoDataToJSONStdOut creates a new StakeInfoDataToJSONStdOut with
// optional existing mutex
func NewStakeInfoDataToJSONStdOut(m ...*sync.Mutex) *StakeInfoDataToJSONStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &StakeInfoDataToJSONStdOut{m[0]}
	}
	return &StakeInfoDataToJSONStdOut{}
}

// NewStakeInfoDataToSummaryStdOut creates a new StakeInfoDataToSummaryStdOut
// with optional existing mutex
func NewStakeInfoDataToSummaryStdOut(m ...*sync.Mutex) *StakeInfoDataToSummaryStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &StakeInfoDataToSummaryStdOut{m[0]}
	}
	return &StakeInfoDataToSummaryStdOut{}
}

// NewStakeInfoDataToJSONFiles creates a new StakeInfoDataToJSONFiles with
// optional existing mutex
func NewStakeInfoDataToJSONFiles(folder string, fileBase string,
	m ...*sync.Mutex) *StakeInfoDataToJSONFiles {
	if len(m) > 1 {
		panic("Too many inputs.")
	}

	var mtx *sync.Mutex
	if len(m) > 0 {
		mtx = m[0]
	} else {
		mtx = new(sync.Mutex)
	}

	return &StakeInfoDataToJSONFiles{
		fileSaver: fileSaver{
			folder:   folder,
			nameBase: fileBase,
			file:     os.File{},
			mtx:      mtx,
		},
	}
}

// Store writes stakeInfoData to stdout in JSON format
func (s *StakeInfoDataToJSONStdOut) Store(data *stakeInfoData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the block data results in to a single JSON object, indented
	jsonConcat, err := JSONFormatStakeInfoData(data)
	if err != nil {
		return err
	}

	fmt.Printf("\n--- BEGIN stakeInfoData JSON ---\n")
	fmt.Println(jsonConcat.String())
	fmt.Printf("--- END stakeInfoData JSON ---\n\n")

	return err
}

// Store writes stakeInfoData to stdout as plain text summary
func (s *StakeInfoDataToSummaryStdOut) Store(data *stakeInfoData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	winSize := activeNet.StakeDiffWindowSize

	fmt.Printf("\nWallet and Stake Info at Height %v:\n", data.height)

	ab := *data.accountBalances

	lenLongestAccount, largestAcctBal := 0, 0.0
	for acct, balances := range ab {
		if len(acct) > lenLongestAccount {
			lenLongestAccount = len(acct)
		}
		if balances.Total > largestAcctBal {
			largestAcctBal = balances.Total
		}
	}

	numDecimals := 4
	maxDigitsAccts := int(1)
	if largestAcctBal > 0 {
		maxDigitsAccts = int(math.Floor(math.Log10(largestAcctBal)) + 1)
	}
	balFmtA := "%" + strconv.Itoa(maxDigitsAccts+numDecimals+2) + "." +
		strconv.Itoa(numDecimals) + "f"

	fmt.Println("- Balances (by account)")
	for acct, balances := range ab {
		padWidth := lenLongestAccount - len(acct) + 2
		if padWidth < 0 {
			padWidth = 0
		}
		padding := strings.Repeat(" ", padWidth)
		fmt.Printf("  %s:%s"+balFmtA+" (any),"+balFmtA+" (spendable),"+balFmtA+
			" (locked),"+balFmtA+" (immat.)\n",
			acct, padding,
			balances.Total, balances.Spendable,
			balances.LockedByTickets,
			balances.ImmatureCoinbaseRewards+balances.ImmatureStakeGeneration)
	}

	maxDigits := int(1)
	if data.balances.AllAllAcounts > 0 {
		maxDigits = int(math.Floor(math.Log10(data.balances.AllAllAcounts)) + 1)
	}
	balFmt := "%" + strconv.Itoa(maxDigits+numDecimals+2) + "." +
		strconv.Itoa(numDecimals) + "f"

	fmt.Println("\n- Balances (by type)")
	fmt.Printf("  spendable:      "+balFmt+" (default),"+balFmt+" (all)\n",
		data.balances.SpendableDefaultAccount,
		data.balances.SpendableAllAccounts)
	fmt.Printf("  locked:         "+balFmt+" (default),"+balFmt+" (all),%10.4f (imported)\n",
		data.balances.LockedDefaultAccount,
		data.balances.LockedAllAccounts,
		data.balances.LockedImportedAccount)
	fmt.Printf("  immat. coinbase:"+balFmt+" (default),"+balFmt+" (all)\n",
		ab["default"].ImmatureCoinbaseRewards,
		data.balances.ImmatureCoinbaseAllAcct)
	fmt.Printf("  immat. votes:   "+balFmt+" (default),"+balFmt+" (all)\n",
		ab["default"].ImmatureStakeGeneration,
		data.balances.ImmatureVotesAllAcct)
	fmt.Printf("  any:            "+balFmt+" (default),"+balFmt+" (all)\n\n",
		data.balances.AllDefaultAcount, data.balances.AllAllAcounts)

	fmt.Println("- Stake Info:")
	fmt.Printf("  Mined tickets:   %4d (immature),%5d (live)\n",
		data.stakeinfo.Immature, data.stakeinfo.Live)

	fmt.Printf("  mempool tickets: %4d (own),     %5d (everyone)\n\n",
		data.stakeinfo.OwnMempoolTix, data.stakeinfo.AllMempoolTix)

	fmt.Printf("      ===>  Voting enabled: %t;  Unlocked: %t  <===\n",
		data.walletInfo.Voting, data.walletInfo.Unlocked)

	fmt.Printf("  Ticket price: %8.3f   |  Window progress: %3d / %3d\n",
		data.stakeinfo.Difficulty, data.idxBlockInWindow, winSize)

	fmt.Printf("  Ticket fee:   %.4f DCR / KB\n",
		data.walletInfo.TicketFee)

	balanceSpendable := data.balances.SpendableAllAccounts
	ticketFee := (550 * data.walletInfo.TicketFee) / 1000
	// TODO: split Tx fee
	splitTxFee := 0.05
	ticketCost := ticketFee + data.stakeinfo.Difficulty + splitTxFee
	fmt.Printf("     (Approximately %.1f tickets may be purchased with set fee.)\n\n",
		balanceSpendable/ticketCost)

	fmt.Printf("  Totals: %8d  votes, %9.2f subsidy\n",
		data.stakeinfo.Voted, data.stakeinfo.TotalSubsidy)
	fmt.Printf("          %8d missed, %9d revoked (%d expired)\n\n",
		data.stakeinfo.Missed, data.stakeinfo.Revoked, data.stakeinfo.Expired)

	return nil
}

// Store writes stakeInfoData to a file in JSON format
// The file name is nameBase+height+".json".
func (s *StakeInfoDataToJSONFiles) Store(data *stakeInfoData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the stake info results in to a single JSON object, indented
	jsonConcat, err := JSONFormatStakeInfoData(data)
	if err != nil {
		return err
	}

	// Write JSON to a file with block height in the name
	height := data.height
	fname := fmt.Sprintf("%s%d.json", s.nameBase, height)
	fullfile := filepath.Join(s.folder, fname)
	fp, err := os.Create(fullfile)
	if err != nil {
		log.Errorf("Unable to open file %v for writing.", fullfile)
		return err
	}
	defer fp.Close()

	s.file = *fp
	//_, err = writeFormattedJSONStakeInfoData(jsonConcat, &s.file)
	_, err = fmt.Fprintln(&s.file, jsonConcat.String())

	return err
}

// JSONFormatStakeInfoData concatenates stake info data results into a single
// JSON object with primary keys for the result type
func JSONFormatStakeInfoData(data *stakeInfoData) (*bytes.Buffer, error) {
	var jsonAll bytes.Buffer

	jsonAll.WriteString("{\"getstakeinfo\": ")
	stakeInfoJSON, err := json.Marshal(data.stakeinfo)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(stakeInfoJSON)
	//stakeInfoJSON, err := json.MarshalIndent(data.stakeinfo, "", "    ")
	//fmt.Println(string(stakeInfoJSON))

	jsonAll.WriteString(",\"walletinfo\": ")
	walletInfoJSON, err := json.Marshal(data.walletInfo)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(walletInfoJSON)

	jsonAll.WriteString(",\"balances\": ")
	balancesJSON, err := json.Marshal(data.balances)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(balancesJSON)

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	err = json.Indent(&jsonAllIndented, jsonAll.Bytes(), "", "    ")
	if err != nil {
		return nil, err
	}

	return &jsonAllIndented, err
}
