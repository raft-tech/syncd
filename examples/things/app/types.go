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
	"crypto"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

func SomethingNew(owner string) *Thing {
	t := &Thing{
		Id:          uuid.New().String(),
		Owner:       owner,
		Name:        adjectives.Rand() + " " + nouns.Rand(),
		ActionCount: 0,
	}
	t.SetVersion()
	return t
}

type Thing struct {
	Id          string
	Owner       string
	Name        string
	Actions     []Action
	ActionCount uint64
	Version     string
}

func (t *Thing) DoSomething() Action {
	a := Action{
		Name:     verbs.Rand(),
		Time:     time.Now(),
		Sequence: uint64(len(t.Actions) + 1),
	}
	t.Actions = append(t.Actions, a)
	t.ActionCount++
	t.SetVersion()
	return a
}

func (t *Thing) SetVersion() {
	md := crypto.SHA1.New()
	if _, e := md.Write([]byte(t.Id + t.Owner + t.Name)); e != nil {
		panic("error writing to SHA1 digest")
	}
	if e := binary.Write(md, binary.BigEndian, t.ActionCount); e != nil {
		panic("error encoding uint64 to binary: " + e.Error())
	}
	sum := make([]byte, 0, md.Size())
	sum = md.Sum(sum)
	t.Version = hex.EncodeToString(sum)[:7]
}

type Action struct {
	Name     string
	Time     time.Time
	Sequence uint64
}

type words []string

func (w words) Rand() string {
	return w[rand.Intn(len(w))]
}

var (
	nouns = words{
		"Turtle",
		"Alligator",
		"Crab",
		"Eel",
		"Crawdad",
		"Catfish",
		"Otter",
		"Snake",
		"Frog",
		"Dolphin",
	}
	adjectives = words{
		"Snappy",
		"Grinning",
		"Clapping",
		"Slippery",
		"Swimming",
		"Silly",
		"Cuddly",
		"Poisonous",
		"Hoppy",
		"Fancy",
	}
	verbs = words{
		"swam",
		"jumped",
		"snapped",
		"ate",
		"flew",
		"ran",
		"clapped",
		"slipped",
	}
)
