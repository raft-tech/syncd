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

import "errors"

type Error interface {
	error
	Code() int
}

func NewError(msg string, exitCode int) Error {
	return &wrappedError{
		error:    errors.New(msg),
		exitCode: exitCode,
	}
}

func WrapError(err error, exitCode int) Error {
	return &wrappedError{
		error:    err,
		exitCode: exitCode,
	}
}

type wrappedError struct {
	error
	exitCode int
}

func (e *wrappedError) Code() int {
	if e == nil {
		return 0
	} else {
		return e.exitCode
	}
}
