/*
 * Copyright (c) 2023. Raft, LLC
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package cmd

import (
	"github.com/raft-tech/syncd/cmd/helpers"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewChecker() *cobra.Command {
	return &cobra.Command{
		RunE:  Check,
		Short: "Check remote peers",
		Use:   "check",
	}
}

func Check(cmd *cobra.Command, args []string) (err error) {

	var config *viper.Viper
	if config, err = helpers.Config(cmd); err != nil {
		return
	}

	var logger *zap.Logger
	if logger, err = helpers.Logger(cmd.OutOrStdout(), config.Sub("logging")); err != nil {
		return
	}

	logger.Info("hello, check!")

	return
}
