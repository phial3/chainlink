package reportcodec

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	relaymercury "github.com/smartcontractkit/chainlink-relay/pkg/plugins/mercury"
	"github.com/smartcontractkit/chainlink/core/logger"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2/types"
)

// NOTE:
// This report codec is based on the original median evmreportcodec
// here:
// https://github.com/smartcontractkit/offchain-reporting/blob/master/lib/offchainreporting2/reportingplugin/median/evmreportcodec/reportcodec.go

var reportTypes = getReportTypes()

func getReportTypes() abi.Arguments {
	mustNewType := func(t string) abi.Type {
		result, err := abi.NewType(t, "", []abi.ArgumentMarshaling{})
		if err != nil {
			panic(fmt.Sprintf("Unexpected error during abi.NewType: %s", err))
		}
		return result
	}
	return abi.Arguments([]abi.Argument{
		{Name: "feedId", Type: mustNewType("bytes32")},
		{Name: "observationsTimestamp", Type: mustNewType("uint32")},
		{Name: "benchmarkPrice", Type: mustNewType("int192")},
		{Name: "bid", Type: mustNewType("int192")},
		{Name: "ask", Type: mustNewType("int192")},
		{Name: "currentBlockNum", Type: mustNewType("uint64")},
		{Name: "currentBlockHash", Type: mustNewType("bytes32")},
		{Name: "validFromBlockNum", Type: mustNewType("uint64")},
	})
}

var _ relaymercury.ReportCodec = &EVMReportCodec{}

type EVMReportCodec struct {
	logger logger.Logger
	feedID [32]byte
}

func NewEVMReportCodec(feedID [32]byte, lggr logger.Logger) *EVMReportCodec {
	return &EVMReportCodec{lggr, feedID}
}

func (r *EVMReportCodec) BuildReport(paos []relaymercury.ParsedAttributedObservation, f int) (ocrtypes.Report, error) {
	if len(paos) == 0 {
		return nil, errors.Errorf("cannot build report from empty attributed observations")
	}

	// copy so we can safely sort in place
	paos = append([]relaymercury.ParsedAttributedObservation{}, paos...)

	timestamp := relaymercury.GetConsensusTimestamp(paos)
	benchmarkPrice := relaymercury.GetConsensusBenchmarkPrice(paos)
	bid := relaymercury.GetConsensusBid(paos)
	ask := relaymercury.GetConsensusAsk(paos)

	currentBlockHash, currentBlockNum, err := relaymercury.GetConsensusCurrentBlock(paos, f)
	if err != nil {
		return nil, errors.Wrap(err, "GetConsensusCurrentBlock failed")
	}

	// get median validFromBlockNum
	validFromBlockNum, err := relaymercury.GetConsensusValidFromBlock(paos)
	if err != nil {
		return nil, errors.Wrap(err, "GetConsensusValidFromBlock failed")
	}

	// Q for Lorenz:
	// TODO: Validate non-zero/non-nil values?
	// TODO: Validate that ValidFromBlockNum <= CurrentBlockNum ?

	reportBytes, err := reportTypes.Pack(r.feedID, timestamp, benchmarkPrice, bid, ask, uint64(currentBlockNum), currentBlockHash, uint64(validFromBlockNum))
	return ocrtypes.Report(reportBytes), errors.Wrap(err, "failed to pack report blob")
}

func bytesToHash(b []byte) (common.Hash, error) {
	if len(b) != 20 {
		return common.Hash{}, errors.Errorf("invalid value for hash: %x", b)
	}
	return common.BytesToHash(b), nil
}

func (r *EVMReportCodec) MaxReportLength(n int) int {
	return 8*32 + // feed ID
		32 + // timestamp
		192 + // benchmarkPrice
		192 + // bid
		192 + // ask
		64 + //currentBlockNum
		8*32 + // currentBlockHash
		64 // validFromBlockNum
}

func (r *EVMReportCodec) CurrentBlockNumFromReport(report ocrtypes.Report) (int64, error) {
	reportElems := map[string]interface{}{}
	if err := reportTypes.UnpackIntoMap(reportElems, report); err != nil {
		return 0, errors.Errorf("error during unpack: %w", err)
	}

	blockNumIface, ok := reportElems["currentBlockNum"]
	if !ok {
		return 0, errors.Errorf("unpacked report has no 'currentBlockNum' field")
	}

	blockNum, ok := blockNumIface.(int64)
	if !ok {
		return 0, errors.Errorf("cannot cast blockNum to int64, type is %T", blockNumIface)
	}

	if blockNum < 0 {
		return 0, errors.Errorf("blockNum may not be negative, got: %d", blockNum)
	}

	return blockNum, nil
}
