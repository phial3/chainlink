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

// TODO: Add [[Mercury.Credentials]] to secrets.toml or something like that for WSRPC

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
	require.NoError(t, err, "Retreiving on-chain wallet addresses for chainlink nodes shouldn't fail")

	// Setup mock server response
	mockServerClient, err := ctfClient.ConnectMockServer(testEnvironment)
	require.NoError(t, err, "Error connecting to mock server")
	// TODO: enable, why it is slow?
	err = mockServerClient.SetValuePath("/constant", 5)
	err = mockServerClient.SetRandomValuePath("/variable")

	require.NoError(t, err, "Setting mockserver value path shouldn't fail")

	t.Cleanup(func() {
		err := actions.TeardownSuite(t, testEnvironment, utils.ProjectRoot, chainlinkNodes, nil, chainClient)
		require.NoError(t, err, "Error tearing down environment")
	})

	// ----- Setup contracts
	contractDeployer, err := contracts.NewContractDeployer(chainClient)
	require.NoError(t, err, "Deploying contracts shouldn't fail")

	// Deploy AccessController for Proxy
	accessController, err := contractDeployer.DeployReadAccessController()
	require.NoError(t, err, "Error deploying ReadAccessController contract")

	// Deploy VeriferProxy
	verifierProxy, err := contractDeployer.DeployVerifierProxy(accessController.Address())
	require.NoError(t, err, "Error deploying VerifierProxy contract")
	_ = verifierProxy

	// Deploy Verifier
	var feedID [32]byte
	copy(feedID[:], "ETH-USD-Optimism-Goerli-1")
	verifier, err := contractDeployer.DeployVerifier(feedID, verifierProxy.Address())
	require.NoError(t, err, "Error deploying Verifier contract")
	nodesWithoutBootstrap := chainlinkNodes[1:]
	// ocrConfig := actions.BuildMercuryOCR2Config(t, nodesWithoutBootstrap, 5*time.Second)
	ocrConfig := actions.Build2(t, nodesWithoutBootstrap)
	verifier.SetConfig(ocrConfig)
	latestConfigDetails, err := verifier.LatestConfigDetails()
	require.NoError(t, err, "Error getting Verifier.LatestConfigDetails()")
	// Init Verifier on the Proxy
	verifierProxy.InitializeVerifier(latestConfigDetails.ConfigDigest, verifier.Address())

	// ----- Create node jobs
	osTemplate := `
		ds1          [type=http method=GET url="%s" allowunrestrictednetworkaccess="true"];
		ds1_parse    [type=jsonparse path="data,result"];
		ds1_multiply [type=multiply times=100];
		ds1 -> ds1_parse -> ds1_multiply -> answer1;

		answer1 [type=median index=0 allowedFaults=4];
	`
	os := fmt.Sprintf(string(osTemplate), mockServerClient.Config.ClusterURL+"/variable")
	network := networks.SelectedNetwork
	CreateMercuryJobs(t, chainlinkNodes, verifier.Address(), feedID, network.ChainID, 0, os)

	// TODO: fix {"level":"error","ts":"2023-02-10T01:08:25.636Z","logger":"OCR","caller":"managed/track_config.go:111","msg":"TrackConfig: LatestConfigDetails() returned a zero configDigest. Looks like the contract has not been configured","contractID":"0x9d97Bf94f5B624A0211c6a005c57bd3D4c3aFc54","jobName":"ocr2","jobID":2,"configDigest":"0000000000000000000000000000000000000000000000000000000000000000","sentryEventID":null,"stacktrace":"github.com/smartcontractkit/libocr/offchainreporting2/internal/managed.(*trackConfigState).checkLatestConfigDetails\n\t/go/pkg/mod/github.com/smartcontractkit/libocr@v0.0.0-20221121171434-482da3ed36d8/offchainreporting2/internal/managed/track_config.go:111\ngithub.com/smartcontractkit/libocr/offchainreporting2/internal/managed.(*trackConfigState).run\n\t/go/pkg/mod/github.com/smartcontractkit/libocr@v0.0.0-20221121171434-482da3ed36d8/offchainreporting2/internal/managed/track_config.go:45\ngithub.com/smartcontractkit/libocr/offchainreporting2/internal/managed.TrackConfig\n\t/go/pkg/mod/github.com/smartcontractkit/libocr@v0.0.0-20221121171434-482da3ed36d8/offchainreporting2/internal/managed/track_config.go:178\ngithub.com/smartcontractkit/libocr/offchainreporting2/internal/managed.(*runWithContractConfigState).run.func1\n\t/go/pkg/mod/github.com/smartcontractkit/libocr@v0.0.0-20221121171434-482da3ed36d8/offchainreporting2/internal/managed/run_with_contract_config.go:67\ngithub.com/smartcontractkit/libocr/subprocesses.(*Subprocesses).Go.func1\n\t/go/pkg/mod/github.com/smartcontractkit/libocr@v0.0.0-20221121171434-482da3ed36d8/subprocesses/subprocesses.go:29"}
	verifier.SetConfig(ocrConfig)
	latestConfigDetails2, err := verifier.LatestConfigDetails()
	_ = latestConfigDetails2

	// Wait for the DON to start generating reports
	time.Sleep(120 * time.Second)

	// Get a report from mercury server
	msUrl := testEnvironment.URLs[mercury_server.URLsKey][1]
	ms := client.NewMercuryServer(msUrl)
	report, _, err := ms.GetReports("ETH-USD-Optimism-Goerli-1", "5554794")
	require.NoError(t, err, "Error getting report from Mercury Server")
	require.NotEmpty(t, report.ChainlinkBlob, "Report response does not contain chainlinkBlob")

	log.Info().Msg("done")
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

	testEnvironment = environment.New(&environment.Config{
		NamespacePrefix: fmt.Sprintf("smoke-mercury-%s", strings.ReplaceAll(strings.ToLower(testNetwork.Name), " ", "-")),
		Test:            t,
	}).
		AddHelm(mockservercfg.New(nil)).
		AddHelm(mockserver.New(nil)).
		AddHelm(evmConfig).
		AddHelm(mercury_server.New(nil)).
		AddHelm(chainlink.New(0, map[string]interface{}{
			"replicas": "5",
			"toml": client.AddNetworksConfig(
				config.BaseMercuryTomlConfig,
				testNetwork),
			"secretsToml": `
				[[Mercury.Credentials]]
				URL = "http://mercury-server:3000/reports"
				Username = "node"
				Password = "nodepass"
			`,
		}))
	err := testEnvironment.Run()
	require.NoError(t, err, "Error running test environment")

	return testEnvironment, testNetwork
}

func CreateMercuryJobs(
	t *testing.T,
	chainlinkNodes []*client.Chainlink,
	contractID string,
	feedID [32]byte,
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
			Name:            "ocr2",
			JobType:         "offchainreporting2",
			MaxTaskDuration: "1s",
			OCR2OracleSpec: job.OCR2OracleSpec{
				PluginType: "median",
				PluginConfig: map[string]interface{}{
					"juelsPerFeeCoinSource": `"""
						bn1          [type=ethgetblock];
						bn1_lookup   [type=lookup key="number"];
						bn1 -> bn1_lookup;
					"""`,
				},
				Relay: "evm",
				RelayConfig: map[string]interface{}{
					"chainID": int(chainID),
				},
				RelayConfigMercuryConfig: map[string]interface{}{
					// "feedID": string(feedID[:]), // TODO: fix transformation
					"feedID": "0x4554482d5553442d4f7074696d69736d2d476f65726c692d3100000000000000",
					"url":    "http://mercury-server:3000/reports", //TODO: use mercury server IP
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
