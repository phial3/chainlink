package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/multierr"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-relay/pkg/loop"
	"github.com/smartcontractkit/chainlink-relay/pkg/types"
	pkgsol "github.com/smartcontractkit/chainlink-solana/pkg/solana"

	"github.com/smartcontractkit/chainlink/core/logger"

	"github.com/smartcontractkit/chainlink/core/chains/solana"
)

func main() {
	logLevelStr := os.Getenv("CL_LOG_LEVEL")
	logLevel, err := zapcore.ParseLevel(logLevelStr)
	if err != nil {
		fmt.Printf("failed to parse CL_LOG_LEVEL = %q: %s\n", logLevelStr, err)
		os.Exit(1)
	}
	cfg := logger.Config{
		LogLevel:    logLevel,
		JsonConsole: strings.EqualFold("true", os.Getenv("CL_JSON_CONSOLE")),
		UnixTS:      strings.EqualFold("true", os.Getenv("CL_UNIX_TS")),
	}
	lggr, closeLggr := cfg.New()
	defer func() {
		if err := closeLggr(); err != nil {
			fmt.Println("Failed to close logger:", err)
		}
	}()
	cp := &chainPlugin{lggr: lggr}
	//TODO graceful shutdown via signal.Notify()? then cp.Close()
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: loop.HandshakeConfig(),
		Plugins: map[string]plugin.Plugin{
			loop.NameChain: loop.NewGRPCChainPlugin(cp, lggr),
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

type chainPlugin struct {
	lggr    logger.Logger
	closers []io.Closer //TODO need to sync
}

func (c *chainPlugin) NewRelayer(ctx context.Context, config string, keystore loop.Keystore) (types.Relayer, error) {
	d := toml.NewDecoder(strings.NewReader(config))
	d.DisallowUnknownFields()
	var cfg solana.SolanaConfig
	if err := d.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config toml: %w", err)
	}

	//TODO ensure nothing else is running? or just not this chain ID?

	chainSet, err := solana.NewChainSetImmut(solana.ChainSetOpts{
		Logger:   c.lggr,
		KeyStore: keystore,
	}, solana.SolanaConfigs{&cfg})
	if err != nil {
		return nil, fmt.Errorf("failed to create chain: %w", err)
	}
	if err := chainSet.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start chain: %w", err)
	}
	r := pkgsol.NewRelayer(c.lggr, chainSet)
	c.closers = append(c.closers, chainSet, r)
	return r, nil
}

func (c *chainPlugin) Close() (err error) {
	for _, cl := range c.closers {
		if e := cl.Close(); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return
}
