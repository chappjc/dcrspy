// Interface for saving/storing blockData and stakeInfoData.
// Create a BlockDataSaver or StakeInfoDataSaver by implementing the
// Store(*blockData) or Store(*stakeInfoData) methods.
//
// chappjc

package main

import (
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
	mtx sync.Mutex
}

// BlockDataToJSONFiles implements BlockDataSaver interface for JSON output to
// the file system
type BlockDataToJSONFiles struct {
	mtx sync.Mutex
}

// BlockDataToMySQL implements BlockDataSaver interface for output to a
// MySQL database
type BlockDataToMySQL struct {
	mtx sync.Mutex
}

// Store writes blockData to stdout in JSON format
func (s *BlockDataToJSONStdOut) Store(data *blockData) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	stakeDiffEstJSON, err := json.MarshalIndent(data.eststakediff, "", "    ")
	fmt.Println(string(stakeDiffEstJSON))

	stakeDiffJSON, err := json.MarshalIndent(data.currentstakediff, "", "    ")
	fmt.Println(string(stakeDiffJSON))

	feeInfoJSON, err := json.MarshalIndent(data.feeinfo, "", "    ")
	fmt.Println(string(feeInfoJSON))

	blockHeaderJSON, err := json.MarshalIndent(data.header, "", "    ")
	fmt.Println(string(blockHeaderJSON))

	poolInfoJSON, err := json.MarshalIndent(data.poolinfo, "", "    ")
	fmt.Println(string(poolInfoJSON))

	return err
}

// StakeInfoDataSaver is an interface for saving/storing stakeInfoData
type StakeInfoDataSaver interface {
	Store(data *stakeInfoData) error
}

// StakeInfoDataToJSONStdOut implements StakeInfoDataSaver interface for JSON
// output to stdout
type StakeInfoDataToJSONStdOut struct {
	mtx sync.Mutex
}

// StakeInfoDataToJSONFiles implements StakeInfoDataSaver interface for JSON
// output to the file system
type StakeInfoDataToJSONFiles struct {
	mtx sync.Mutex
}

// StakeInfoDataToMySQL implements StakeInfoDataSaver interface for output to a
// MySQL database
type StakeInfoDataToMySQL struct {
	mtx sync.Mutex
}

// Store writes stakeInfoData to stdout in JSON format
func (s *StakeInfoDataToJSONStdOut) Store(data *stakeInfoData) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	stakeInfoJSON, err := json.MarshalIndent(data.stakeinfo, "", "    ")
	fmt.Println(string(stakeInfoJSON))

	return err
}
