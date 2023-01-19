// config is a separate package so that we can validate
// the config in other packages, for example in job at job create time.

package config

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"

	"github.com/smartcontractkit/chainlink/core/store/models"
)

type PluginConfig struct {
	FeedID          common.Hash   `json:"feedID"`
	URL             *models.URL   `json:"url"`
	ServerPubKey    hexutil.Bytes `json:"serverPubKey"`
	ClientPrivKeyID string        `json:"clientPrivKeyID"`
}

func ValidatePluginConfig(config PluginConfig) error {
	if config.URL == nil {
		return errors.New("Mercury URL must be specified")
	}
	if (config.FeedID == common.Hash{}) {
		return errors.New("FeedID must be specified and non-zero")
	}
	// TODO: Check config.TransmitterID, it should be valid and present OCR2 public key
	return nil
}
