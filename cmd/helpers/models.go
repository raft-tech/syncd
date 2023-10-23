/*
 *     Copyright (c) 2023. Raft LLC
 *
 *     This program is free software: you can redistribute it and/or modify
 *     it under the terms of the GNU General Public License as published by
 *     the Free Software Foundation, either version 3 of the License, or
 *     (at your option) any later version.
 *
 *     This program is distributed in the hope that it will be useful,
 *     but WITHOUT ANY WARRANTY; without even the implied warranty of
 *     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *     GNU General Public License for more details.
 *
 *     You should have received a copy of the GNU General Public License
 *     along with this program.  If not, see <https://www.gnu.org/licenses/>.
 *
 */

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
