package transportmgr

import (
	"github.com/kaleido-io/paladin/toolkit/pkg/retry"
	"github.com/kaleido-io/paladin/kata/internal/plugins"
	"gopkg.in/yaml.v3"
)

type TransportManagerConfig struct {
	Transports map[string]*TransportConfig `yaml:"transports"`
}

type TransportInitConfig struct {
	Retry retry.Config `yaml:"retry"`
}

type TransportConfig struct {
	Init          TransportInitConfig  `yaml:"init"`
	Plugin        plugins.PluginConfig `yaml:"plugin"`
	Config        yaml.Node            `yaml:"config"`
}