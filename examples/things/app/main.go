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

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "things [FLAGS]",
		Version: "v1.0.0",
		RunE:    ExecuteC,
	}
	cmd.PersistentFlags().StringP("conn", "c", "postgres://localhost:5432/public", "PostgreSQL connection string")
	cmd.PersistentFlags().DurationP("every", "w", 10*time.Second, "Duration between generations")
	cmd.PersistentFlags().UintP("updates", "n", 10, "Number of times to update each record")
	return cmd
}

type config struct {
	Connection string        `mapstructure:"conn"`
	Duration   time.Duration `mapstructure:"every"`
	Updates    uint
}

func ExecuteC(cmd *cobra.Command, args []string) error {

	ctx := cmd.Context()

	var owner string
	if i := len(args); i == 1 {
		owner = strings.TrimSpace(args[0])
	} else {
		return errors.New(fmt.Sprintf("expected one argument, was %d", i))
	}

	var cfg config
	{
		v := viper.New()
		if err := v.BindPFlags(cmd.Flags()); err == nil {
			v.SetEnvPrefix("SYNCD_THINGS")
			v.AutomaticEnv()
			if err = v.Unmarshal(&cfg); err != nil {
				return nil
			}
		} else {
			return err
		}
	}

	var db *pgxpool.Pool
	if c, err := pgxpool.ParseConfig(cfg.Connection); err == nil {
		if db, err = pgxpool.NewWithConfig(ctx, c); err == nil {
			defer db.Close()
		} else {
			return err
		}
	}

	things := make([]*Thing, 0, cfg.Updates)
	lock := sync.Mutex{}
	updating := errors.New("starting record processing")
	counter := new(atomic.Uint64)

	// Prepare to generate records
	logger := log.New(cmd.OutOrStdout(), "", log.LstdFlags)
	logger.Println("starting updates")
	ticker := time.NewTicker(cfg.Duration)

	// Log an update after every update or every 5 sec (whichever is longer)
	go func() {
		var total uint64
		d := 5 * time.Second
		if d < cfg.Duration {
			d = cfg.Duration
		}
		logTicker := time.NewTicker(d)
		for done := false; !done; {
			select {
			case <-logTicker.C:
				n := counter.Swap(0)
				total += n
				logger.Printf("%d records processed (%d total)", n, total)
			case <-ctx.Done():
				done = true
			}
		}
	}()

	// Do the work
	update := func() error {

		if lock.TryLock() {
			defer lock.Unlock()
		} else {
			return updating
		}

		var conn *pgxpool.Conn
		if c, err := db.Acquire(ctx); err == nil {
			conn = c
			defer c.Release()
		} else {
			return err
		}

		// Shift things right, then and something new
		idx := int(cfg.Updates) - 1
		if l := len(things); l < idx {
			idx = l
		}
		for i := 0; i < idx; i++ {

			thing := things[i]

			// Start a transaction
			var tx pgx.Tx
			if t, err := conn.BeginTx(ctx, pgx.TxOptions{}); err == nil {
				tx = t
				defer func() {
					_ = tx.Rollback(ctx)
				}()
			} else {
				return err
			}
			if rows, e := tx.Query(ctx, "SELECT id FROM things.things WHERE id = $1 FOR UPDATE", thing.Id); e != nil {
				return e
			} else if !rows.Next() {
				if e = rows.Err(); e != nil {
					return e
				} else {
					return errors.New(fmt.Sprintf("thing %s missing from database", thing.Id))
				}
			} else {
				rows.Close()
			}

			// Add an action
			act := thing.DoSomething()
			if _, err := tx.Exec(ctx, "INSERT INTO things.actions (thing, action, time, sequence) VALUES ($1, $2, $3, $4)", thing.Id, act.Name, act.Time.Unix(), act.Sequence); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE things.things SET actionCount = $2, version = $3 WHERE id = $1", thing.Id, thing.ActionCount, thing.Version); err != nil {
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			counter.Add(1)
		}

		// Add something new
		thing := SomethingNew(owner)
		act := thing.DoSomething()
		var tx pgx.Tx
		if t, err := conn.BeginTx(ctx, pgx.TxOptions{}); err == nil {
			tx = t
			defer func() {
				_ = t.Rollback(ctx)
			}()
		} else {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO things.things (id, owner, name, actionCount, version) VALUES ($1, $2, $3, $4, $5)", thing.Id, owner, thing.Name, thing.ActionCount, thing.Version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO things.actions (thing, action, time, sequence) VALUES ($1, $2, $3, $4)", thing.Id, act.Name, act.Time.Unix(), act.Sequence); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		counter.Add(1)

		// Shift right, add the new thing
		if idx == 0 {
			things = append(things, thing)
		} else {
			i := idx - 1
			things = append(things, things[i])
			for ; i > 0; i-- {
				things[i] = things[i-1]
			}
			things[0] = thing
		}

		return nil
	}
	for done := false; !done; {

		if err := update(); err != nil {
			switch {
			case errors.Is(err, updating):
				logger.Println("updates in progress; skipping this instance")
			case errors.Is(err, context.Canceled):
				done = true
				continue
			default:
				logger.Printf("error occurred: %s", err.Error())
			}
		}

		select {
		case <-ctx.Done():
			done = true
			ticker.Stop()
		case _ = <-ticker.C:
		}
	}
	logger.Println("shutdown complete")
	return nil
}

func main() {

	// Create a context that is canceled by OS signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Run the root command
	if err := New().ExecuteContext(ctx); err != nil {
		code := 1
		if err, ok := err.(interface{ Code() int }); ok {
			code = err.Code()
		}
		os.Exit(code)
	}
}
