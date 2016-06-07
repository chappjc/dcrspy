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
	"sync"
	//"github.com/decred/dcrd/dcrjson"
	//"github.com/decred/dcrutil"
)

// BlockDataSaver is an interface for saving/storing blockData
type BlockDataSaver interface {
	Store(data *blockData) error
}

// BlockDataToJSONStdOut implements BlockDataSaver interface for JSON output to
// stdout
type BlockDataToJSONStdOut struct {
	mtx *sync.Mutex
}

// BlockDataToJSONFiles implements BlockDataSaver interface for JSON output to
// the file system
type BlockDataToJSONFiles struct {
	mtx *sync.Mutex
}

// BlockDataToMySQL implements BlockDataSaver interface for output to a
// MySQL database
type BlockDataToMySQL struct {
	mtx *sync.Mutex
}

// NewBlockDataToJSONStdOut creates a new BlockDataToJSONStdOut with optional
// existing mutex
func NewBlockDataToJSONStdOut(m ...*sync.Mutex) *BlockDataToJSONStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &BlockDataToJSONStdOut{m[0]}
	}
	return &BlockDataToJSONStdOut{new(sync.Mutex)}
}

// Store writes blockData to stdout in JSON format
func (s *BlockDataToJSONStdOut) Store(data *blockData) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var jsonAll bytes.Buffer

	jsonAll.WriteString("{\"estimatestakediff\": ")
	stakeDiffEstJSON, err := json.Marshal(data.eststakediff)
	jsonAll.Write(stakeDiffEstJSON)
	//stakeDiffEstJSON, err := json.MarshalIndent(data.eststakediff, "", "    ")
	//fmt.Println(string(stakeDiffEstJSON))

	jsonAll.WriteString(",\"currentstakediff\": ")
	stakeDiffJSON, err := json.Marshal(data.currentstakediff)
	jsonAll.Write(stakeDiffJSON)
	//stakeDiffJSON, err := json.MarshalIndent(data.currentstakediff, "", "    ")
	//fmt.Println(string(stakeDiffJSON))

	jsonAll.WriteString(",\"ticketfeeinfo_block\": ")
	feeInfoJSON, err := json.Marshal(data.feeinfo)
	jsonAll.Write(feeInfoJSON)
	//feeInfoJSON, err := json.MarshalIndent(data.feeinfo, "", "    ")
	//fmt.Println(string(feeInfoJSON))

	jsonAll.WriteString(",\"block_header\": ")
	blockHeaderJSON, err := json.Marshal(data.header)
	jsonAll.Write(blockHeaderJSON)
	//blockHeaderJSON, err := json.MarshalIndent(data.header, "", "    ")
	//fmt.Println(string(blockHeaderJSON))

	jsonAll.WriteString(",\"ticket_pool_info\": ")
	poolInfoJSON, err := json.Marshal(data.poolinfo)
	jsonAll.Write(poolInfoJSON)
	//poolInfoJSON, err := json.MarshalIndent(data.poolinfo, "", "    ")
	//fmt.Println(string(poolInfoJSON))

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	json.Indent(&jsonAllIndented,jsonAll.Bytes(),"","    ")

	fmt.Printf("\n--- BEGIN blockData JSON ---\n")
	fmt.Println(jsonAllIndented.String())
	fmt.Printf("--- END blockData JSON ---\n\n")

	return err
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

// StakeInfoDataToJSONFiles implements StakeInfoDataSaver interface for JSON
// output to the file system
type StakeInfoDataToJSONFiles struct {
	mtx *sync.Mutex
}

// StakeInfoDataToMySQL implements StakeInfoDataSaver interface for output to a
// MySQL database
type StakeInfoDataToMySQL struct {
	mtx *sync.Mutex
}

// NewStakeInfoDataToJSONStdOut creates a new StakeInfoDataToJSONStdOut with optional
// existing mutex
func NewStakeInfoDataToJSONStdOut(m ...*sync.Mutex) *StakeInfoDataToJSONStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &StakeInfoDataToJSONStdOut{m[0]}
	}
	return &StakeInfoDataToJSONStdOut{new(sync.Mutex)}
}

// Store writes stakeInfoData to stdout in JSON format
func (s *StakeInfoDataToJSONStdOut) Store(data *stakeInfoData) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var jsonAll bytes.Buffer

	jsonAll.WriteString("{\"getstakeinfo\": ")
	stakeInfoJSON, err := json.Marshal(data.stakeinfo)
	jsonAll.Write(stakeInfoJSON)
	//stakeInfoJSON, err := json.MarshalIndent(data.stakeinfo, "", "    ")
	//fmt.Println(string(stakeInfoJSON))

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	json.Indent(&jsonAllIndented,jsonAll.Bytes(),"","    ")

	fmt.Printf("\n--- BEGIN stakeInfoData JSON ---\n")
	fmt.Println(jsonAllIndented.String())
	fmt.Printf("--- END stakeInfoData JSON ---\n\n")

	return err
}
