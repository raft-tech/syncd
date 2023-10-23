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

package helpers

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Config(cmd *cobra.Command) (*viper.Viper, error) {

	if cmd == nil {
		panic("cmd must not be nil")
	}

	cfg := viper.New()

	// Support env variables prefixed with SYNCD_
	cfg.SetEnvPrefix("syncd")
	cfg.AutomaticEnv()

	// Load config from file
	var src []string
	if s := os.Getenv("SYNCD_CONFIG"); s != "" {
		src = []string{s}
	}
	if s, _ := cmd.Flags().GetStringSlice("config"); len(s) > 0 {
		src = s
	}
	for _, s := range src {
		cfg.SetConfigFile(s)
		if err := cfg.MergeInConfig(); err != nil {
			return nil, WrapError(err, 2)
		}
	}

	if err := cfg.BindPFlags(cmd.Flags()); err != nil {
		return nil, WrapError(err, 2)
	}

	return cfg, nil
}

type StringValue struct {
	Value   string
	FromEnv string
}

func (sv *StringValue) GetValue() string {
	var val string
	if val = sv.Value; val == "" {
		if e := sv.FromEnv; e != "" {
			val = os.Getenv(e)
		}
	}
	return val
}
