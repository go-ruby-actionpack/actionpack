// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package controller

// filter is one entry in a before/after/around callback chain with its
// only/except/if/unless conditions.
type filter struct {
	fn        ActionFunc
	around    func(c *Base, yield func() error) error
	only      map[string]bool
	except    map[string]bool
	ifCond    func(c *Base) bool
	unlessErr func(c *Base) bool
}

// applies reports whether the filter runs for the current action.
func (f *filter) applies(b *Base) bool {
	a := b.action
	if len(f.only) > 0 && !f.only[a] {
		return false
	}
	if len(f.except) > 0 && f.except[a] {
		return false
	}
	if f.ifCond != nil && !f.ifCond(b) {
		return false
	}
	if f.unlessErr != nil && f.unlessErr(b) {
		return false
	}
	return true
}

// FilterOption constrains when a filter runs.
type FilterOption func(*filter)

// Only restricts a filter to the listed actions.
func Only(actions ...string) FilterOption {
	return func(f *filter) { f.only = toSet(actions) }
}

// Except runs a filter for every action but the listed ones.
func Except(actions ...string) FilterOption {
	return func(f *filter) { f.except = toSet(actions) }
}

// If runs a filter only when cond returns true.
func If(cond func(c *Base) bool) FilterOption {
	return func(f *filter) { f.ifCond = cond }
}

// Unless runs a filter only when cond returns false.
func Unless(cond func(c *Base) bool) FilterOption {
	return func(f *filter) { f.unlessErr = cond }
}

func toSet(actions []string) map[string]bool {
	m := make(map[string]bool, len(actions))
	for _, a := range actions {
		m[a] = true
	}
	return m
}

// BeforeAction registers a before filter, returning the class for chaining. A
// before filter that renders or redirects halts the chain.
func (cl *Class) BeforeAction(fn ActionFunc, opts ...FilterOption) *Class {
	cl.before = append(cl.before, newFilter(fn, opts))
	return cl
}

// AfterAction registers an after filter.
func (cl *Class) AfterAction(fn ActionFunc, opts ...FilterOption) *Class {
	cl.after = append(cl.after, newFilter(fn, opts))
	return cl
}

// AroundAction registers an around filter. The filter must call yield to run
// the remaining chain and the action; not calling yield skips them.
func (cl *Class) AroundAction(fn func(c *Base, yield func() error) error, opts ...FilterOption) *Class {
	f := filter{around: fn}
	for _, o := range opts {
		o(&f)
	}
	cl.around = append(cl.around, f)
	return cl
}

func newFilter(fn ActionFunc, opts []FilterOption) filter {
	f := filter{fn: fn}
	for _, o := range opts {
		o(&f)
	}
	return f
}
