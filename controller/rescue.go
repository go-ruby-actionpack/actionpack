// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package controller

import "errors"

// rescuer pairs an error matcher with its handler, the analogue of a
// rescue_from declaration.
type rescuer struct {
	match  func(err error) bool
	handle func(c *Base, err error) error
}

// RescueFrom registers a handler for errors matching match. Later registrations
// take precedence, mirroring rescue_from's last-declared-wins ordering.
func (cl *Class) RescueFrom(match func(err error) bool, handle func(c *Base, err error) error) *Class {
	cl.rescuers = append(cl.rescuers, rescuer{match: match, handle: handle})
	return cl
}

// RescueFromIs registers a handler for errors that wrap the sentinel target
// (matched with errors.Is).
func (cl *Class) RescueFromIs(target error, handle func(c *Base, err error) error) *Class {
	return cl.RescueFrom(func(err error) bool { return errors.Is(err, target) }, handle)
}

// RescueType registers a handler for errors of the concrete type T (matched
// with errors.As), the closest Go analogue of rescue_from SomeError. Example:
// RescueType[*parameters.ParameterMissing](cl, handler).
func RescueType[T error](cl *Class, handle func(c *Base, err error) error) *Class {
	return cl.RescueFrom(func(err error) bool {
		var t T
		return errors.As(err, &t)
	}, handle)
}
