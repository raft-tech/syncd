package helpers

import (
	"github.com/raft-tech/syncd/pkg/graph"
	"github.com/spf13/viper"
)

func Models(cfg *viper.Viper) (ModelMap, error) {
	var models ModelMap
	if err := cfg.Unmarshal(&models); err != nil {
		return nil, err
	}
	return models, nil
}

type Model struct {
	Filters []graph.Filter
}

type ModelMap map[string]Model
