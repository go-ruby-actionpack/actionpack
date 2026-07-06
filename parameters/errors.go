// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parameters

import (
	"sort"
	"strings"
)

// ParameterMissing is the analogue of ActionController::ParameterMissing,
// raised by Require when a required key is absent or blank.
type ParameterMissing struct {
	// Key is the missing parameter name.
	Key string
	// Params lists the keys that were present, for diagnostics.
	Params []string
}

func (e *ParameterMissing) Error() string {
	return "param is missing or the value is empty or invalid: " + e.Key
}

// UnpermittedParameters is the analogue of
// ActionController::UnpermittedParameters, used when
// ActionOnUnpermittedParameters is "raise".
type UnpermittedParameters struct {
	// Keys are the parameter names that were present but not permitted.
	Keys []string
}

func (e *UnpermittedParameters) Error() string {
	keys := make([]string, len(e.Keys))
	copy(keys, e.Keys)
	sort.Strings(keys)
	return "found unpermitted parameter" + plural(len(keys)) + ": " +
		strings.Join(keys, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// UnfilteredParameters is the analogue of
// ActionController::UnfilteredParameters, returned by ToH on parameters that
// have not been permitted.
type UnfilteredParameters struct{}

func (e *UnfilteredParameters) Error() string {
	return "unable to convert unpermitted parameters to hash"
}
