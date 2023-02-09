package contracts

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink/integration-tests/contracts/ethereum/mercury/exchanger"
	"github.com/smartcontractkit/chainlink/integration-tests/contracts/ethereum/mercury/verifier_proxy"
)

type Exchanger interface {
	Address() string
}

type EthereumExchanger struct {
	address   *common.Address
	client    blockchain.EVMClient
	exchanger *exchanger.Exchanger
}

func (v *EthereumExchanger) Address() string {
	return v.address.Hex()
}

type VerifierProxy interface {
	Address() string
}

type EthereumVerifierProxy struct {
	address       *common.Address
	client        blockchain.EVMClient
	verifierProxy *verifier_proxy.VerifierProxy
}

func (v *EthereumVerifierProxy) Address() string {
	return v.address.Hex()
}

func (e *EthereumContractDeployer) DeployVerifierProxy(accessControllerAddr string) (VerifierProxy, error) {
	address, _, instance, err := e.client.DeployContract("VerifierProxy", func(
		auth *bind.TransactOpts,
		backend bind.ContractBackend,
	) (common.Address, *types.Transaction, interface{}, error) {
		return verifier_proxy.DeployVerifierProxy(auth, backend, common.HexToAddress(accessControllerAddr))
	})
	if err != nil {
		return nil, err
	}
	return &EthereumVerifierProxy{
		client:        e.client,
		address:       address,
		verifierProxy: instance.(*verifier_proxy.VerifierProxy),
	}, err
}

func (e *EthereumContractDeployer) DeployExchanger(verifierProxyAddr string, lookupURL string, maxDelay uint8) (Exchanger, error) {
	address, _, instance, err := e.client.DeployContract("Exchanger", func(
		auth *bind.TransactOpts,
		backend bind.ContractBackend,
	) (common.Address, *types.Transaction, interface{}, error) {
		return exchanger.DeployExchanger(auth, backend,
			common.HexToAddress(verifierProxyAddr), lookupURL, maxDelay)
	})
	if err != nil {
		return nil, err
	}
	return &EthereumExchanger{
		client:    e.client,
		address:   address,
		exchanger: instance.(*exchanger.Exchanger),
	}, err
}
