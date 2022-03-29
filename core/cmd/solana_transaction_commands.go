package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.uber.org/multierr"

	"github.com/smartcontractkit/chainlink/core/store/models/solana"
	"github.com/smartcontractkit/chainlink/core/web/presenters"
)

// SolanaSendSol transfers sol from the node's account to a specified address.
func (cli *Client) SolanaSendSol(c *cli.Context) (err error) {
	if c.NArg() < 3 {
		return cli.errorOut(errors.New("three arguments expected: amount, fromAddress and toAddress"))
	}

	amount, err := strconv.ParseUint(c.Args().Get(0), 10, 64)
	if err != nil {
		return cli.errorOut(fmt.Errorf("invalid amount: %w", err))
	}

	unparsedFromAddress := c.Args().Get(1)
	fromAddress, err := solanaGo.PublicKeyFromBase58(unparsedFromAddress)
	if err != nil {
		return cli.errorOut(multierr.Combine(
			errors.Errorf("while parsing withdrawal source address %v",
				unparsedFromAddress), err))
	}

	unparsedDestinationAddress := c.Args().Get(2)
	destinationAddress, err := solanaGo.PublicKeyFromBase58(unparsedDestinationAddress)
	if err != nil {
		return cli.errorOut(multierr.Combine(
			errors.Errorf("while parsing withdrawal destination address %v",
				unparsedDestinationAddress), err))
	}

	chainID := c.String("id")
	if chainID == "" {
		return cli.errorOut(errors.New("missing id"))
	}

	request := solana.SendRequest{
		To:                 destinationAddress,
		From:               fromAddress,
		Amount:             amount,
		SolanaChainID:      chainID,
		AllowHigherAmounts: c.IsSet("force"),
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return cli.errorOut(err)
	}

	buf := bytes.NewBuffer(requestData)

	resp, err := cli.HTTP.Post("/v2/transfers/solana", buf)
	if err != nil {
		return cli.errorOut(err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			err = multierr.Append(err, cerr)
		}
	}()

	err = cli.renderAPIResponse(resp, &presenters.SolanaMsgResource{})
	return err
}
