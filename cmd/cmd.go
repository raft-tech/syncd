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
	"github.com/spf13/cobra"
)

var Version string

func New() *cobra.Command {
	cmd := &cobra.Command{
		SilenceErrors: true,
		SilenceUsage:  true,
		Use:           "syncd [FLAGS] COMMAND",
		Version:       Version,
	}
	cmd.PersistentFlags().StringSlice("config", nil, "path/to/config.yaml")
	cmd.PersistentFlags().String("logging.level", "info", "one of: debug, info, warn, error, fatal")
	cmd.PersistentFlags().String("logging.format", "text", "one of: text, json")
	cmd.AddCommand(NewPush(), NewPull(), NewServer())
	return cmd
}
