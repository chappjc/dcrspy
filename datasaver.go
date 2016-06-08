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
	"os"
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
	nameBase string
	file     os.File
	mtx      *sync.Mutex
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
	return &BlockDataToJSONStdOut{}
}

// NewBlockDataToJSONFiles creates a new BlockDataToJSONFiles with optional
// existing mutex
func NewBlockDataToJSONFiles(fileBase string, m ...*sync.Mutex) *BlockDataToJSONFiles {
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
		nameBase: fileBase,
		file:     os.File{},
		mtx:      mtx,
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
	fp, err := os.Create(fname)
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

// StakeInfoDataToJSONFiles implements StakeInfoDataSaver interface for JSON
// output to the file system
type StakeInfoDataToJSONFiles struct {
	nameBase string
	file     os.File
	mtx      *sync.Mutex
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
	return &StakeInfoDataToJSONStdOut{}
}

// NewStakeInfoDataToJSONFiles creates a new StakeInfoDataToJSONFiles with optional
// existing mutex
func NewStakeInfoDataToJSONFiles(fileBase string, m ...*sync.Mutex) *StakeInfoDataToJSONFiles {
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
		nameBase: fileBase,
		file:     os.File{},
		mtx:      mtx,
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
	fp, err := os.Create(fname)
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

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	err = json.Indent(&jsonAllIndented, jsonAll.Bytes(), "", "    ")
	if err != nil {
		return nil, err
	}

	return &jsonAllIndented, err
}
