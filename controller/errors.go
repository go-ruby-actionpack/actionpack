// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package controller

// ActionNotFound is returned by Process when the action has no registered
// ActionFunc and the class has no RunAction seam, the analogue of
// AbstractController::ActionNotFound.
type ActionNotFound struct {
	Controller string
	Action     string
}

func (e *ActionNotFound) Error() string {
	return "controller: the action '" + e.Action +
		"' could not be found for " + e.Controller
}

// DoubleRenderError is returned when a render/redirect/head is attempted after
// the response was already produced, the analogue of
// AbstractController::DoubleRenderError.
type DoubleRenderError struct{}

func (e *DoubleRenderError) Error() string {
	return "controller: render and/or redirect were called multiple times " +
		"in this action"
}
