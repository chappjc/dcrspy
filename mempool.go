package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	//"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

var resetMempoolTix bool

type mempoolInfo struct {
	currentHeight               uint32
	numTicketPurchasesInMempool uint32
	numTicketsSinceStatsReport  int32
	lastCollectTime             time.Time
}

type mempoolMonitor struct {
	mpoolInfo      mempoolInfo
	newTicketLimit int32
	minInterval    time.Duration
	maxInterval    time.Duration
	collector      *mempoolDataCollector
	dataSavers     []MempoolDataSaver
	newTxChan      <-chan *chainhash.Hash
	quit           chan struct{}
	wg             *sync.WaitGroup
	mtx            sync.RWMutex
}

// newMempoolMonitor creates a new mempoolMonitor
func newMempoolMonitor(collector *mempoolDataCollector,
	txChan <-chan *chainhash.Hash, savers []MempoolDataSaver,
	quit chan struct{}, wg *sync.WaitGroup, newTicketLimit int32,
	mini time.Duration, maxi time.Duration) *mempoolMonitor {
	return &mempoolMonitor{
		mpoolInfo:      mempoolInfo{}, // defaults OK here?
		newTicketLimit: newTicketLimit,
		minInterval:    mini,
		maxInterval:    maxi,
		collector:      collector,
		dataSavers:     savers,
		newTxChan:      txChan,
		quit:           quit,
		wg:             wg,
	}
}

func (p *mempoolMonitor) txHandler(client *dcrrpcclient.Client) {
	defer p.wg.Done()
	//out:
	for {
		//keepon:
		select {
		case s, ok := <-p.newTxChan:
			if !ok {
				mempoolLog.Infof("New Tx channel closed")
				return
			}

			var err error
			var txHeight uint32
			var oneTicket int32

			// See if this was just the ticker firing
			if s.IsEqual(new(chainhash.Hash)) {
				// Just the ticker
				bestBlock, err := client.GetBlockCount()
				if err != nil {
					mempoolLog.Error("Unable to get block count")
					continue
				}
				txHeight = uint32(bestBlock)
				// proceed in case it has been quiteLong
			} else {
				// OnTxAccepted probably sent on newTxChan
				tx, err := client.GetRawTransaction(s)
				if err != nil {
					mempoolLog.Errorf("Failed to get transaction %v: %v",
						s.String(), err)
					continue
				}

				mempoolLog.Trace("New Tx: ", tx.Sha(), tx.MsgTx().TxIn[0].ValueIn)

				// See dcrd/blockchain/stake/staketx.go for information about
				// specifications for different transaction types (TODO).
				txType := stake.DetermineTxType(tx)
				//s.Tree() == dcrutil.TxTreeRegular

				var ticketHash *chainhash.Hash

				switch txType {
				case stake.TxTypeRegular:
					// Regular Tx
					mempoolLog.Tracef("Received regular transaction: %v", tx.Sha())
					continue
				case stake.TxTypeSStx:
					// Ticket purchase
					ticketHash = tx.Sha()
				case stake.TxTypeSSGen:
					// Vote
					ticketHash = &tx.MsgTx().TxIn[1].PreviousOutPoint.Hash
					mempoolLog.Tracef("Received vote %v for ticket %v", tx.Sha(), ticketHash)
					// TODO: Show subsidy for this vote (Vout[2] - Vin[1] ?)
					continue
				case stake.TxTypeSSRtx:
					// Revoke
					mempoolLog.Tracef("Received revoke transaction: %v", tx.Sha())
					continue
				default:
					// Unknown
					mempoolLog.Warnf("Received other transaction: %v", tx.Sha())
					continue
				}

				price := tx.MsgTx().TxOut[0].Value
				mempoolLog.Tracef("Received ticket purchase %v, price %v", ticketHash,
					dcrutil.Amount(price).ToCoin())
				// TODO: Get fee for this ticket (Vin[0] - Vout[0])

				txHeight = tx.MsgTx().TxIn[0].BlockHeight
				oneTicket = 1
			}

			// s.server.txMemPool.TxDescs()
			ticketHashes, err := client.GetRawMempool(dcrjson.GRMTickets)
			if err != nil {
				mempoolLog.Errorf("Could not get raw mempool: %v", err.Error())
				continue
			}
			N := len(ticketHashes)
			p.mpoolInfo.numTicketPurchasesInMempool = uint32(N)

			// Decide if it is time to collect and record new data
			// 1. Get block height
			// 2. Record num new and total tickets in mp
			// 3. Collect mempool info (fee info), IF ANY OF:
			//   a. block is new (height of Ticket-Tx > currentHeight)
			//   b. num new tickets >= newTicketLimit
			//   c. time since last exceeds > maxInterval
			//   AND
			//   time since lastCollectTime is at least minInterval
			p.mtx.Lock()
			// Atomics really aren't necessary here because of mutex
			newBlock := txHeight > p.mpoolInfo.currentHeight
			enoughNewTickets := atomic.AddInt32(&p.mpoolInfo.numTicketsSinceStatsReport, oneTicket) >= p.newTicketLimit
			timeSinceLast := time.Since(p.mpoolInfo.lastCollectTime)
			quiteLong := timeSinceLast > p.maxInterval
			longEnough := timeSinceLast >= p.minInterval

			if newBlock {
				atomic.StoreUint32(&p.mpoolInfo.currentHeight, txHeight)
			}

			newTickets := p.mpoolInfo.numTicketsSinceStatsReport

			var data *mempoolData
			if (newBlock || enoughNewTickets || quiteLong) && longEnough {
				atomic.StoreInt32(&p.mpoolInfo.numTicketsSinceStatsReport, 0)
				p.mpoolInfo.lastCollectTime = time.Now()
				p.mtx.Unlock()
				mempoolLog.Trace("Gathering new mempool data.")
				data, err = p.collector.collect()
				if err != nil {
					mempoolLog.Errorf("mempool data collection failed: %v", err.Error())
					// data is nil when err != nil
					continue
				}
			} else {
				p.mtx.Unlock()
				continue
			}

			data.newTickets = uint32(newTickets)

			//p.mpoolInfo.numTicketPurchasesInMempool = data.ticketfees.FeeInfoMempool.Number

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
			mempoolLog.Debugf("Quitting OnRecvTx/OnRedeemingTx handler.")
			return
		}
	}
}

// TODO
func (p *mempoolMonitor) maybeCollect(txHeight uint32) (*mempoolData, error) {
	p.mtx.Lock()
	newBlock := txHeight > p.mpoolInfo.currentHeight
	enoughNewTickets := atomic.AddInt32(&p.mpoolInfo.numTicketsSinceStatsReport, 1) > p.newTicketLimit
	timeSinceLast := time.Since(p.mpoolInfo.lastCollectTime)
	quiteLong := timeSinceLast > p.maxInterval
	longEnough := timeSinceLast > p.minInterval

	if newBlock {
		atomic.StoreUint32(&p.mpoolInfo.currentHeight, txHeight)
	}

	var err error
	var data *mempoolData
	if (newBlock || enoughNewTickets || quiteLong) && longEnough {
		p.mpoolInfo.lastCollectTime = time.Now()
		p.mtx.Unlock()
		mempoolLog.Infof("Gathering new mempool data.")
		data, err = p.collector.collect()
		if err != nil {
			mempoolLog.Errorf("mempool data collection failed: %v", err.Error())
			// data is nil when err != nil
		}
	} else {
		p.mtx.Unlock()
	}

	return data, err
}

// COLLECTOR

type minableFeeInfo struct {
	all               []float64
	lowestMineableIdx int
	lowestMineableFee float64
	targetFeeWindow   []float64
}

type mempoolData struct {
	height      uint32
	numTickets  uint32
	newTickets  uint32
	ticketfees  *dcrjson.TicketFeeInfoResult
	minableFees *minableFeeInfo
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
		mempoolLog.Tracef("mempoolDataCollector.collect() completed in %v", time.Since(start))
	}(time.Now())

	// client
	c := t.dcrdChainSvr

	// mempoolTickets[ticketHashes[0].String()].Fee
	mempoolTickets, err := c.GetRawMempoolVerbose(dcrjson.GRMTickets)
	N := len(mempoolTickets)
	allFees := make([]float64, 0, N)
	for _, t := range mempoolTickets {
		txSize := float64(t.Size)
		allFees = append(allFees, t.Fee/txSize*1000)
	}
	//medianFee := MedianCoin(allFees)
	//mempoolLog.Infof("Median fee computed: %v (%v)", medianFee, N)

	Nmax := int(activeChain.MaxFreshStakePerBlock)
	sort.Float64s(allFees)
	var lowestMineableFee float64
	var lowestMineableIdx int
	if N >= Nmax {
		lowestMineableIdx = N - Nmax
		lowestMineableFee = allFees[lowestMineableIdx]
	} else if N == 0 {
		lowestMineableIdx = -1
	}

	var targetFeeWindow []float64
	if N > 0 {
		lowEnd := lowestMineableIdx - 5
		if lowEnd < 0 {
			lowEnd = 0
		}
		highEnd := lowestMineableIdx + 5
		if highEnd > N {
			highEnd = N
		}
		targetFeeWindow = allFees[lowEnd:highEnd]
	}

	mineables := &minableFeeInfo{
		allFees,
		lowestMineableIdx,
		lowestMineableFee,
		targetFeeWindow,
	}

	height, err := c.GetBlockCount()

	// Fee info
	numFeeBlocks := uint32(0)
	numFeeWindows := uint32(0)

	feeInfo, err := c.TicketFeeInfo(&numFeeBlocks, &numFeeWindows)
	if err != nil {
		return nil, err
	}

	//feeInfoMempool := feeInfo.FeeInfoMempool

	mpoolData := &mempoolData{
		height:      uint32(height),
		numTickets:  feeInfo.FeeInfoMempool.Number,
		ticketfees:  feeInfo,
		minableFees: mineables,
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
	mtx             *sync.Mutex
	feeWindowRadius int
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
func NewMempoolDataToSummaryStdOut(feeWindowRadius int, m ...*sync.Mutex) *MempoolDataToSummaryStdOut {
	if len(m) > 1 {
		panic("Too many inputs.")
	}
	if len(m) > 0 {
		return &MempoolDataToSummaryStdOut{m[0], feeWindowRadius}
	}
	return &MempoolDataToSummaryStdOut{nil, feeWindowRadius}
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
	// Do not write JSON data if there are no new tickets since last report
	if data.newTickets == 0 {
		return nil
	}

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
	if err != nil {
		mempoolLog.Error("Write JSON mempool data to stdout pipe: ", os.Stdout)
	}

	return err
}

// Store writes mempoolData to stdout as plain text summary
func (s *MempoolDataToSummaryStdOut) Store(data *mempoolData) error {
	if s.mtx != nil {
		s.mtx.Lock()
		defer s.mtx.Unlock()
	}

	mempoolTicketFees := data.ticketfees.FeeInfoMempool

	// time.Now().UTC().Format(time.UnixDate)
	_, err := fmt.Printf("%v - Mempool ticket fees (%v):  %.4f, %.4f, %.4f, %.4f (l/m, mean, median, std), n=%d\n",
		time.Now().Format("2006-01-02 15:04:05.00 -0700 MST"), data.height,
		data.minableFees.lowestMineableFee,
		mempoolTicketFees.Mean, mempoolTicketFees.Median,
		mempoolTicketFees.StdDev, mempoolTicketFees.Number)

	boundIdx := data.minableFees.lowestMineableIdx
	N := len(data.minableFees.all)

	var upperFees, lowerFees []float64
	w := s.feeWindowRadius
	if N > 2 {
		lowEnd := boundIdx - w
		if lowEnd < 0 {
			lowEnd = 0
		}
		highEnd := boundIdx + w + 1 // +1 for slice indexing
		if highEnd > N {
			highEnd = N
		}

		lowerFees = data.minableFees.all[lowEnd:boundIdx]
		upperFees = data.minableFees.all[boundIdx+1 : highEnd]
	}

	if N > 1 {
		fmt.Printf("Mineable tickets, limit -%d/+%d:\t%.5f --> %.5f (threshold) --> %.5f\n",
			len(lowerFees), len(upperFees), lowerFees, data.minableFees.lowestMineableFee, upperFees)
	}

	return err
}

// Store writes mempoolData to a file in JSON format
// The file name is nameBase+height+".json".
func (s *MempoolDataToJSONFiles) Store(data *mempoolData) error {
	// Do not write JSON data if there are no new tickets since last report
	if data.newTickets == 0 {
		return nil
	}

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
	fname := fmt.Sprintf("%s%d-%d.json", s.nameBase, data.height,
		data.numTickets)
	fullfile := filepath.Join(s.folder, fname)
	fp, err := os.Create(fullfile)
	defer fp.Close()
	if err != nil {
		mempoolLog.Errorf("Unable to open file %v for writting.", fullfile)
		return err
	}

	s.file = *fp
	_, err = writeFormattedJSONMempoolData(jsonConcat, &s.file)
	if err != nil {
		mempoolLog.Error("Write JSON mempool data to file: ", *fp)
	}

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
		mempoolLog.Error("Unable to marshall mempool ticketfee info to JSON: ",
			err.Error())
		return nil, err
	}

	jsonAll.Write(feeInfoMempoolJSON)
	//feeInfoMempoolJSON, err := json.MarshalIndent(data.ticketfees.FeeInfoMempool, "", "    ")
	//fmt.Println(string(feeInfoMempoolJSON))

	jsonAll.WriteString("}")

	var jsonAllIndented bytes.Buffer
	err = json.Indent(&jsonAllIndented, jsonAll.Bytes(), "", "    ")
	if err != nil {
		mempoolLog.Error("Unable to format JSON mempool data: ", err.Error())
		return nil, err
	}

	return &jsonAllIndented, err
}

// MedianAmount gets the median Amount from a slice of Amounts
func MedianAmount(s []dcrutil.Amount) dcrutil.Amount {
	if len(s) == 0 {
		return 0
	}

	sort.Sort(dcrutil.AmountSorter(s))

	middle := len(s) / 2

	if len(s) == 0 {
		return 0
	} else if (len(s) % 2) != 0 {
		return s[middle]
	}
	return (s[middle] + s[middle-1]) / 2
}

// MedianCoin gets the median DCR from a slice of float64s
func MedianCoin(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}

	sort.Float64s(s)

	middle := len(s) / 2

	if len(s) == 0 {
		return 0
	} else if (len(s) % 2) != 0 {
		return s[middle]
	}
	return (s[middle] + s[middle-1]) / 2
}
