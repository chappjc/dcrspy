
package main

import (
    "bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
    "time"
    //"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

var resetMempoolTix bool

type mempoolInfo struct {
    currentHeight                   uint32
    numTicketPurchasesInMempool     int32
    numTicketsSinceStatsReport      int32
	lastCollectTime					time.Time
}

type mempoolMonitor struct {
    mpoolInfo          mempoolInfo
	newTicketCountLogTrigger int32
	minInterval		   time.Duration
	maxInterval		   time.Duration
    collector          *mempoolDataCollector
	dataSavers         []MempoolDataSaver
	newTxChan          <-chan *dcrutil.Tx
	quit               chan struct{}
	wg                 *sync.WaitGroup
    mtx                 sync.RWMutex
}

// newMempoolMonitor creates a new mempoolMonitor
func newMempoolMonitor(collector *mempoolDataCollector,
	txChan <-chan *dcrutil.Tx, savers []MempoolDataSaver,
	quit chan struct{}, wg *sync.WaitGroup) *mempoolMonitor {
	return &mempoolMonitor{
		collector:          collector,
		dataSavers:         savers,
		newTxChan:          txChan,
		quit:               quit,
		wg:                 wg,
	}
}

func (p *mempoolMonitor) txHandler () {
	defer p.wg.Done()
//out:
    for {
	keepon:
        select {
        case s, ok := <-p.newTxChan:
            if !ok {
                log.Infof("New Tx channel closed")
                return
            }
            //tx, err := dcrdClient.GetRawTransaction(s.Sha())
            // if err != nil {
            //     log.Errorf("Failed to get transaction %v: %v",
            //         s.Sha().String(), err)
            //     return
            // }
            log.Info("New Tx.  TxIn[0]: ", s.MsgTx().TxIn[0])
			
			txHeight := s.MsgTx().TxIn[0].BlockHeight

			// TODO IMPORTANT: check if the tx is a ticket purchase
			// TODO: get number of tickets in mempool, store in numTicketPurchasesInMempool
			//       update numTicketsSinceStatsReport that way instead of +1

			p.mtx.Lock()
			defer p.mtx.Unlock()
            // 1. Get block height
			// 2. Record num new and total tickets in mp 
			// 3. Collect mempool info (fee info), IF ANY OF:
			//   a. block is new (height of Ticket-Tx > currentHeight)
			//   b. num new tickets >= newTicketCountLogTrigger
			//   c. time since last exceeds > maxInterval
			//   AND
			//   time since lastCollectTime is greater than minInterval
			newBlock := txHeight > p.mpoolInfo.currentHeight
			enoughNewTickets := atomic.AddInt32(&p.mpoolInfo.numTicketsSinceStatsReport,1) > p.newTicketCountLogTrigger
			timeSinceLast := time.Since(p.mpoolInfo.lastCollectTime)
			quiteLong := timeSinceLast > p.minInterval
			longEnough := timeSinceLast > p.minInterval

			if newBlock {
				p.mpoolInfo.currentHeight = txHeight
			}

			var data *mempoolData 
			var err error
			if (newBlock || enoughNewTickets || quiteLong) && longEnough {
				data, err = p.collector.collect()
				if err != nil {
					log.Errorf("mempool data collection failed: %v", err.Error())
					// data is nil when err != nil
					break keepon
				}
				p.mpoolInfo.lastCollectTime = time.Now()
			}

			// Store block data with each saver
			for _, s := range p.dataSavers {
				if s != nil {
					// save data to wherever the saver wants to put it
					go s.Store(data)
				}
			}

		// case s, ok := <-newTicketChan:
        //     if !ok {
        //         log.Infof("New ticket channel closed")
        //         return
        //     }
        case <-p.quit:
            log.Debugf("Quitting OnRecvTx/OnRedeemingTx handler.")
            return
        }
    }
}


// COLLECTOR

type mempoolData struct {
    height      uint32
    ticketfees  *dcrjson.TicketFeeInfoResult
}


type mempoolDataCollector struct {
	mtx          sync.Mutex
	cfg          *config
	dcrdChainSvr *dcrrpcclient.Client
}

// newMempoolDataCollector creates a new mempoolDataCollector.
func newMempoolDataCollector(cfg *config,
	dcrdChainSvr *dcrrpcclient.Client) (*mempoolDataCollector, error) {
	return &mempoolDataCollector{
		mtx:          sync.Mutex{},
		cfg:          cfg,
		dcrdChainSvr: dcrdChainSvr,
	}, nil
}

// collect is the main handler for collecting chain data
func (t *mempoolDataCollector) collect() (*mempoolData, error) {
	// In case of a very fast block, make sure previous call to collect is not
	// still running, or dcrd may be mad.
	t.mtx.Lock()
	defer t.mtx.Unlock()

	// Time this function
	defer func(start time.Time) {
		log.Debugf("mempoolDataCollector.collect() completed in %v", time.Since(start))
	}(time.Now())

    height, err := t.dcrdChainSvr.GetBlockCount()

	// Fee info
	numFeeBlocks := uint32(0)
	numFeeWindows := uint32(0)

	feeInfo, err := t.dcrdChainSvr.TicketFeeInfo(&numFeeBlocks, &numFeeWindows)
	if err != nil {
		return nil, err
	}

	//feeInfoMempool := feeInfo.FeeInfoMempool

    mpoolData := &mempoolData{
        height:     uint32(height),
        ticketfees: feeInfo,
    }

    return mpoolData, err
}


// SAVER

// MempoolDataSaver is an interface for saving/storing mempoolData
type MempoolDataSaver interface {
	Store(data *mempoolData) error
}

// MempoolDataToJSONStdOut implements MempoolDataSaver interface for JSON output to
// stdout
type MempoolDataToJSONStdOut struct {
	mtx *sync.Mutex
}

// MempoolDataToSummaryStdOut implements MempoolDataSaver interface for plain text
// summary to stdout
type MempoolDataToSummaryStdOut struct {
	mtx *sync.Mutex
}

// MempoolDataToJSONFiles implements MempoolDataSaver interface for JSON output to
// the file system
type MempoolDataToJSONFiles struct {
	fileSaver
}

// MempoolDataToMySQL implements MempoolDataSaver interface for output to a
// MySQL database
type MempoolDataToMySQL struct {
	mtx *sync.Mutex
}

// NewMempoolDataToJSONStdOut creates a new MempoolDataToJSONStdOut with optional
// existing mutex
func NewMempoolDataToJSONStdOut(m ...*sync.Mutex) *MempoolDataToJSONStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &MempoolDataToJSONStdOut{m[0]}
	}
	return &MempoolDataToJSONStdOut{}
}

// NewMempoolDataToSummaryStdOut creates a new MempoolDataToSummaryStdOut with optional
// existing mutex
func NewMempoolDataToSummaryStdOut(m ...*sync.Mutex) *MempoolDataToSummaryStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &MempoolDataToSummaryStdOut{m[0]}
	}
	return &MempoolDataToSummaryStdOut{}
}

// NewMempoolDataToJSONFiles creates a new MempoolDataToJSONFiles with optional
// existing mutex
func NewMempoolDataToJSONFiles(folder string, fileBase string, m ...*sync.Mutex) *MempoolDataToJSONFiles {
	if len(m) > 1 {
		panic("Too many inputs.")
	}

	var mtx *sync.Mutex
	if len(m) > 0 {
		mtx = m[0]
	} else {
		mtx = new(sync.Mutex)
	}

	return &MempoolDataToJSONFiles{
		fileSaver: fileSaver{
			folder:   folder,
			nameBase: fileBase,
			file:     os.File{},
			mtx:      mtx,
		},
	}
}

// Store writes mempoolData to stdout in JSON format
func (s *MempoolDataToJSONStdOut) Store(data *mempoolData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the block data results in to a single JSON object, indented
	jsonConcat, err := JSONFormatMempoolData(data)
	if err != nil {
		return err
	}

	// Write JSON to stdout with guards to delimit the object from other text
	fmt.Printf("\n--- BEGIN mempoolData JSON ---\n")
	_, err = writeFormattedJSONMempoolData(jsonConcat, os.Stdout)
	fmt.Printf("--- END mempoolData JSON ---\n\n")

	return err
}

// Store writes mempoolData to stdout as plain text summary
func (s *MempoolDataToSummaryStdOut) Store(data *mempoolData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	//winSize := activeNet.StakeDiffWindowSize

    mempoolTicketFees := data.ticketfees.FeeInfoMempool

	fmt.Printf("\nBlock %v:\n", data.height)

	var err error
	_, err = fmt.Printf("\tTicket fees:  %.4f, %.4f, %.4f (mean, median, std), n=%d\n",
		mempoolTicketFees.Mean, mempoolTicketFees.Median, mempoolTicketFees.StdDev,
        mempoolTicketFees.Number)

	return err
}

// Store writes mempoolData to a file in JSON format
// The file name is nameBase+height+".json".
func (s *MempoolDataToJSONFiles) Store(data *mempoolData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	// Marshall all the block data results in to a single JSON object, indented
	jsonConcat, err := JSONFormatMempoolData(data)
	if err != nil {
		return err
	}

	// Write JSON to a file with block height in the name
	fname := fmt.Sprintf("%s%d.json", s.nameBase, data.height)
	fullfile := filepath.Join(s.folder, fname)
	fp, err := os.Create(fullfile)
	defer fp.Close()
	if err != nil {
		log.Errorf("Unable to open file %v for writting.", fullfile)
		return err
	}

	s.file = *fp
	_, err = writeFormattedJSONMempoolData(jsonConcat, &s.file)

	return err
}

func writeFormattedJSONMempoolData(jsonConcat *bytes.Buffer, w io.Writer) (int, error) {
	n, err := fmt.Fprintln(w, jsonConcat.String())
	// there was once more, perhaps again.
	return n, err
}

// JSONFormatMempoolData concatenates block data results into a single JSON
// object with primary keys for the result type
func JSONFormatMempoolData(data *mempoolData) (*bytes.Buffer, error) {
	var jsonAll bytes.Buffer

	jsonAll.WriteString("{\"ticketfeeinfo_mempool\": ")
	feeInfoMempoolJSON, err := json.Marshal(data.ticketfees.FeeInfoMempool)
	if err != nil {
		return nil, err
	}
	jsonAll.Write(feeInfoMempoolJSON)
	//feeInfoMempoolJSON, err := json.MarshalIndent(data.ticketfees.FeeInfoMempool, "", "    ")
	//fmt.Println(string(feeInfoMempoolJSON))

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	err = json.Indent(&jsonAllIndented, jsonAll.Bytes(), "", "    ")
	if err != nil {
		return nil, err
	}

	return &jsonAllIndented, err
}
