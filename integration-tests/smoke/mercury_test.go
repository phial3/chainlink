package smoke

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/chainlink"
	eth "github.com/smartcontractkit/chainlink-env/pkg/helm/ethereum"
	mercury_server "github.com/smartcontractkit/chainlink-env/pkg/helm/mercury-server"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver"
	mockservercfg "github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver-cfg"
	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	ctfClient "github.com/smartcontractkit/chainlink-testing-framework/client"
	"github.com/smartcontractkit/chainlink-testing-framework/utils"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink/core/services/job"
	"github.com/smartcontractkit/chainlink/core/services/keystore/chaintype"
	"github.com/smartcontractkit/chainlink/core/store/models"
	networks "github.com/smartcontractkit/chainlink/integration-tests"

	"github.com/smartcontractkit/chainlink/integration-tests/actions"
	"github.com/smartcontractkit/chainlink/integration-tests/client"
	"github.com/smartcontractkit/chainlink/integration-tests/config"
	"github.com/smartcontractkit/chainlink/integration-tests/contracts"
)

func TestMercury(t *testing.T) {
	t.Parallel()
	testEnvironment, testNetwork := setupMercuryEnvironment(t)
	if testEnvironment.WillUseRemoteRunner() {
		return
	}

	chainClient, err := blockchain.NewEVMClient(testNetwork, testEnvironment)
	require.NoError(t, err, "Error connecting to blockchain")
	chainlinkNodes, err := client.ConnectChainlinkNodes(testEnvironment)
	require.NoError(t, err, "Error connecting to Chainlink nodes")
	nodeAddresses, err := actions.ChainlinkNodeAddresses(chainlinkNodes)
	_ = nodeAddresses
	require.NoError(t, err, "Retreiving on-chain wallet addresses for chainlink nodes shouldn't fail")

	t.Cleanup(func() {
		err := actions.TeardownSuite(t, testEnvironment, utils.ProjectRoot, chainlinkNodes, nil, chainClient)
		require.NoError(t, err, "Error tearing down environment")
	})

	// Setup mock server response
	mockServerClient, err := ctfClient.ConnectMockServer(testEnvironment)
	require.NoError(t, err, "Error connecting to mock server")
	err = mockServerClient.SetValuePath("/variable", 5)
	require.NoError(t, err, "Setting mockserver value path shouldn't fail")

	// Setup jobs
	// Create jobs
	osTemplate := `
		ds1          [type=http method=GET url="%s" allowunrestrictednetworkaccess="true"];
		ds1_parse    [type=jsonparse path="answer"];
		ds1_multiply [type=multiply times=100];
		ds1 -> ds1_parse -> ds1_multiply -> answer1;

		answer1 [type=median index=0 allowedFaults=4];
	`
	os := fmt.Sprintf(string(osTemplate), mockServerClient.Config.ClusterURL+"/variable")
	// TODO: use verifier contract id
	contractId := "0x57947C1ef9F56373A2f7361c63Be6deeaf7cB3b1"
	network := networks.SelectedNetwork
	CreateOCR2Jobs(t, chainlinkNodes, contractId, network.ChainID, 0, os)

	// Setup contracts

	contractDeployer, err := contracts.NewContractDeployer(chainClient)
	require.NoError(t, err, "Deploying contracts shouldn't fail")

	// Deploy AccessController for Proxy
	accessController, err := contractDeployer.DeployReadAccessController()
	require.NoError(t, err, "Error deploying ReadAccessController contract")

	// Deploy VeriferProxy
	verifierProxy, err := contractDeployer.DeployVerifierProxy(accessController.Address())
	require.NoError(t, err, "Error deploying VerifierProxy contract")
	_ = verifierProxy
}

func setupMercuryEnvironment(t *testing.T) (testEnvironment *environment.Environment, testNetwork blockchain.EVMNetwork) {
	testNetwork = networks.SelectedNetwork
	evmConfig := eth.New(nil)
	if !testNetwork.Simulated {
		evmConfig = eth.New(&eth.Props{
			NetworkName: testNetwork.Name,
			Simulated:   testNetwork.Simulated,
			WsURLs:      testNetwork.URLs,
		})
	}

	// TODO: setup DON with OCR2
	testEnvironment = environment.New(&environment.Config{
		NamespacePrefix: fmt.Sprintf("smoke-mercury-%s", strings.ReplaceAll(strings.ToLower(testNetwork.Name), " ", "-")),
		Test:            t,
	}).
		AddHelm(evmConfig).
		AddHelm(mockservercfg.New(nil)).
		AddHelm(mockserver.New(nil)).
		AddHelm(mercury_server.New(nil)).
		AddHelm(chainlink.New(0, map[string]interface{}{
			"replicas": "5",
			"toml": client.AddNetworksConfig(
				config.BaseMercuryTomlConfig,
				testNetwork),
		}))
	err := testEnvironment.Run()

	require.NoError(t, err, "Error running test environment")

	return testEnvironment, testNetwork
}

func CreateOCR2Jobs(
	t *testing.T,
	chainlinkNodes []*client.Chainlink,
	contractID string,
	chainID int64,
	keyIndex int,
	observationSource string,
) {
	bootstrapNode := chainlinkNodes[0]
	bootstrapNode.RemoteIP()
	bootstrapP2PIds, err := bootstrapNode.MustReadP2PKeys()
	require.NoError(t, err, "Shouldn't fail reading P2P keys from bootstrap node")
	bootstrapP2PId := bootstrapP2PIds.Data[0].Attributes.PeerID

	bootstrapSpec := &client.OCR2TaskJobSpec{
		Name:    "ocr2 bootstrap node",
		JobType: "bootstrap",
		OCR2OracleSpec: job.OCR2OracleSpec{
			ContractID: contractID,
			Relay:      "evm",
			RelayConfig: map[string]interface{}{
				"chainID": int(chainID),
			},
			ContractConfigTrackerPollInterval: *models.NewInterval(time.Second * 15),
		},
	}
	_, err = bootstrapNode.MustCreateJob(bootstrapSpec)
	require.NoError(t, err, "Shouldn't fail creating bootstrap job on bootstrap node")
	P2Pv2Bootstrapper := fmt.Sprintf("%s@%s:%d", bootstrapP2PId, bootstrapNode.RemoteIP(), 6690)

	for nodeIndex := 1; nodeIndex < len(chainlinkNodes); nodeIndex++ {
		nodeTransmitterAddress, err := chainlinkNodes[nodeIndex].EthAddresses()
		require.NoError(t, err, "Shouldn't fail getting primary ETH address from OCR node %d", nodeIndex+1)
		nodeOCRKeys, err := chainlinkNodes[nodeIndex].MustReadOCR2Keys()
		require.NoError(t, err, "Shouldn't fail getting OCR keys from OCR node %d", nodeIndex+1)
		var nodeOCRKeyId []string
		for _, key := range nodeOCRKeys.Data {
			if key.Attributes.ChainType == string(chaintype.EVM) {
				nodeOCRKeyId = append(nodeOCRKeyId, key.ID)
				break
			}
		}

		autoOCR2JobSpec := client.OCR2TaskJobSpec{
			Name:    "ocr2",
			JobType: "offchainreporting2",
			OCR2OracleSpec: job.OCR2OracleSpec{
				PluginType: "median",
				Relay:      "evm",
				RelayConfig: map[string]interface{}{
					"chainID": int(chainID),
				},
				ContractConfigTrackerPollInterval: *models.NewInterval(time.Second * 15),
				ContractID:                        contractID,                                        // registryAddr
				OCRKeyBundleID:                    null.StringFrom(nodeOCRKeyId[keyIndex]),           // get node ocr2config.ID
				TransmitterID:                     null.StringFrom(nodeTransmitterAddress[keyIndex]), // node addr
				P2PV2Bootstrappers:                pq.StringArray{P2Pv2Bootstrapper},                 // bootstrap node key and address <p2p-key>@bootstrap:8000
			},
			ObservationSource: observationSource,
		}

		_, err = chainlinkNodes[nodeIndex].MustCreateJob(&autoOCR2JobSpec)
		require.NoError(t, err, "Shouldn't fail creating OCR Task job on OCR node %d", nodeIndex+1)
	}
	log.Info().Msg("Done creating OCR automation jobs")
}
