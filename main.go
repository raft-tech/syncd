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

package main

import (
	"context"
	_ "embed"
	"os"
	"os/signal"

	"github.com/raft-tech/syncd/cmd"
)

//go:embed VERSION
var version string

func init() {
	cmd.Version = version
}

func main() {

	// Create a context that is canceled by OS signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Run the root command
	if err := cmd.New().ExecuteContext(ctx); err != nil {
		code := 1
		if err, ok := err.(interface{ Code() int }); ok {
			code = err.Code()
		}
		os.Exit(code)
	}
}
