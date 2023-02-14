package mercury

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/pkg/errors"
	relaymercury "github.com/smartcontractkit/chainlink-relay/pkg/plugins/mercury"
	"github.com/smartcontractkit/libocr/offchainreporting2/chains/evmutil"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2/types"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services"
	"github.com/smartcontractkit/chainlink/core/services/relay/evm/mercury/wsrpc"
	pb "github.com/smartcontractkit/chainlink/core/services/relay/evm/mercury/wsrpc/report"
)

type Transmitter interface {
	relaymercury.Transmitter
	services.ServiceCtx
}

var _ Transmitter = &mercuryTransmitter{}

type mercuryTransmitter struct {
	lggr      logger.Logger
	rpcClient wsrpc.Client

	reportURL string
}

var payloadTypes = getPayloadTypes()

func getPayloadTypes() abi.Arguments {
	mustNewType := func(t string) abi.Type {
		result, err := abi.NewType(t, "", []abi.ArgumentMarshaling{})
		if err != nil {
			panic(fmt.Sprintf("Unexpected error during abi.NewType: %s", err))
		}
		return result
	}
	return abi.Arguments([]abi.Argument{
		{Name: "reportContext", Type: mustNewType("bytes32[3]")},
		{Name: "report", Type: mustNewType("bytes")},
		{Name: "rawRs", Type: mustNewType("bytes32[]")},
		{Name: "rawSs", Type: mustNewType("bytes32[]")},
		{Name: "rawVs", Type: mustNewType("bytes32")},
	})
}

func NewTransmitter(lggr logger.Logger, rpcClient wsrpc.Client, reportURL string) *mercuryTransmitter {
	return &mercuryTransmitter{lggr.Named("Mercury"), rpcClient, reportURL}
}

func (mt *mercuryTransmitter) Start(ctx context.Context) error { return mt.rpcClient.Start(ctx) }
func (mt *mercuryTransmitter) Close() error                    { return mt.rpcClient.Close() }
func (mt *mercuryTransmitter) Healthy() error                  { return mt.rpcClient.Healthy() }
func (mt *mercuryTransmitter) Ready() error                    { return mt.rpcClient.Ready() }

// Transmit sends the report to the on-chain smart contract's Transmit method.
func (mt *mercuryTransmitter) Transmit(ctx context.Context, reportCtx ocrtypes.ReportContext, report ocrtypes.Report, signatures []ocrtypes.AttributedOnchainSignature) error {
	var rs [][32]byte
	var ss [][32]byte
	var vs [32]byte
	for i, as := range signatures {
		r, s, v, err := evmutil.SplitSignature(as.Signature)
		if err != nil {
			panic("eventTransmit(ev): error in SplitSignature")
		}
		rs = append(rs, r)
		ss = append(ss, s)
		vs[i] = v
	}
	rawReportCtx := evmutil.RawReportContext(reportCtx)

	payload, err := payloadTypes.Pack(rawReportCtx, []byte(report), rs, ss, vs)
	if err != nil {
		return errors.Wrap(err, "abi.Pack failed")
	}

	rr := &pb.ReportRequest{
		Payload: payload,
	}

	mt.lggr.Debugw("Transmitting report", "reportRequest", rr, "report", report, "reportCtx", reportCtx, "signatures", signatures)

	res, err := mt.rpcClient.Transmit(ctx, rr)
	if err != nil {
		return errors.Wrap(err, "failed to POST to mercury server")
	}

	if res.Error == "" {
		mt.lggr.Debugw("Transmit report success", "response", res, "reportCtx", reportCtx)
	} else {
		mt.lggr.Errorw("Transmit report failed", "response", res, "reportCtx", reportCtx)

	}

	return nil
}

// TODO: Should return CSA pub key actually
func (mt *mercuryTransmitter) FromAccount() ocrtypes.Account {
	return ocrtypes.Account("TODO")
}

// LatestConfigDigestAndEpoch retrieves the latest config digest and epoch from the OCR2 contract.
// It is plugin independent, in particular avoids use of the plugin specific generated evm wrappers
// by using the evm client Call directly for functions/events that are part of OCR2Abstract.
func (mt *mercuryTransmitter) LatestConfigDigestAndEpoch(ctx context.Context) (cd ocrtypes.ConfigDigest, epoch uint32, err error) {
	// ConfigDigest and epoch are not stored on the contract in mercury mode
	// TODO: Need to implement this for Mercury
	return errors.New("TODO")
}

func (mt *mercuryTransmitter) FetchInitialMaxFinalizedBlockNumber(context.Context) (int64, error) {
	// TODO: Need to implement this for Mercury
	return 0, errors.New("TODO")
}
