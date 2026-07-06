// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package controller is a pure-Go, CGO-free port of the AbstractController and
// ActionController::Metal dispatch core: a controller class model with action
// dispatch, before/after/around filters (with only/except/if/unless
// conditions), rescue_from error handling, and render/redirect_to/head as
// seams over the deferred view layer.
//
// Action bodies and view rendering are Ruby seams. An action is either a
// registered ActionFunc or is handled by the class-wide RunAction seam
// (mapping to Rails' `def <action>; ...; end`). Rendering is produced by an
// optional Renderer seam; without one, render records its options and writes
// its :plain body, leaving template rendering to a later ActionView port.
package controller

import (
	"github.com/go-ruby-actionpack/actionpack/dispatch"
	"github.com/go-ruby-actionpack/actionpack/parameters"
)

// ActionFunc is the body of a controller action (a seam for Ruby code).
type ActionFunc func(c *Base) error

// Renderer is the view-layer seam. It receives the controller and the render
// options and returns the rendered body. When nil, render falls back to the
// :plain option.
type Renderer func(c *Base, opts RenderOptions) (string, error)

// Class is the controller class: the shared configuration (name, action seams,
// filter chains, rescuers, renderer) from which per-request Base instances are
// created. It is the union of AbstractController::Base and
// ActionController::Metal in one value.
type Class struct {
	Name string

	actions   map[string]ActionFunc
	runAction func(c *Base, action string) error
	renderer  Renderer

	before []filter
	after  []filter
	around []filter

	rescuers []rescuer
}

// NewClass returns a controller class with the given name (e.g. "posts").
func NewClass(name string) *Class {
	return &Class{Name: name, actions: map[string]ActionFunc{}}
}

// Action registers the body seam for a named action, returning the class for
// chaining.
func (cl *Class) Action(name string, fn ActionFunc) *Class {
	cl.actions[name] = fn
	return cl
}

// RunAction sets the fallback dispatch seam invoked for any action without a
// registered ActionFunc. This is the primary Ruby seam: it receives the Base
// (params, request, response, and the render/redirect/head methods) and the
// action name, mirroring ActionController's `process(action)` reaching the
// Ruby action method.
func (cl *Class) RunAction(fn func(c *Base, action string) error) *Class {
	cl.runAction = fn
	return cl
}

// SetRenderer installs the view-layer seam.
func (cl *Class) SetRenderer(r Renderer) *Class {
	cl.renderer = r
	return cl
}

// Base is a controller instance handling one request.
type Base struct {
	class     *Class
	action    string
	params    *parameters.Parameters
	request   *dispatch.Request
	response  *dispatch.Response
	performed bool

	// Rendered records the options of the render that produced the response,
	// or nil when the response came from redirect_to/head.
	Rendered *RenderOptions
	// RedirectedTo records the target of a redirect_to, or "".
	RedirectedTo string
}

// New builds a controller instance for a request. Any of req/resp/params may be
// nil; a nil response is replaced with a fresh empty one and nil params with an
// empty set.
func (cl *Class) New(req *dispatch.Request, resp *dispatch.Response, params *parameters.Parameters) *Base {
	if resp == nil {
		resp = dispatch.NewResponse()
	}
	if params == nil {
		params = parameters.New(nil)
	}
	return &Base{class: cl, params: params, request: req, response: resp}
}

// Class returns the controller class.
func (b *Base) Class() *Class { return b.class }

// Action returns the action name currently being processed.
func (b *Base) Action() string { return b.action }

// Params returns the request parameters.
func (b *Base) Params() *parameters.Parameters { return b.params }

// SetParams replaces the parameters (e.g. after require/permit).
func (b *Base) SetParams(p *parameters.Parameters) { b.params = p }

// Request returns the dispatch request (may be nil).
func (b *Base) Request() *dispatch.Request { return b.request }

// Response returns the dispatch response.
func (b *Base) Response() *dispatch.Response { return b.response }

// Performed reports whether a render/redirect/head has produced the response.
func (b *Base) Performed() bool { return b.performed }

// Process dispatches the action through the filter chain and rescue handlers,
// mirroring ActionController::Metal#process. It returns the (possibly rescued)
// error from the action or a filter.
func (b *Base) Process(action string) error {
	b.action = action
	return b.withRescue(func() error {
		return b.runCallbacks(func() error { return b.invokeAction(action) })
	})
}

// invokeAction runs the action seam: a registered ActionFunc, else the class
// RunAction seam, else ActionNotFound.
func (b *Base) invokeAction(action string) error {
	if fn, ok := b.class.actions[action]; ok {
		return fn(b)
	}
	if b.class.runAction != nil {
		return b.class.runAction(b, action)
	}
	return &ActionNotFound{Controller: b.class.Name, Action: action}
}

// withRescue runs fn, routing a returned error or a panicked error value
// through rescue_from handlers.
func (b *Base) withRescue(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = b.rescue(e)
				return
			}
			panic(r)
		}
	}()
	if e := fn(); e != nil {
		return b.rescue(e)
	}
	return nil
}

// rescue dispatches err to the first matching rescuer, or returns it unhandled.
func (b *Base) rescue(err error) error {
	for _, r := range b.rescuersReversed() {
		if r.match(err) {
			return r.handle(b, err)
		}
	}
	return err
}

// rescuersReversed returns rescuers most-recently-registered first, matching
// Rails' rescue_from precedence (last declared wins).
func (b *Base) rescuersReversed() []rescuer {
	src := b.class.rescuers
	out := make([]rescuer, len(src))
	for i := range src {
		out[i] = src[len(src)-1-i]
	}
	return out
}

// runCallbacks runs before filters (halting the chain when one performs a
// render/redirect), then the around/action stack, then after filters. A halted
// before filter skips the action and the after filters, matching ActiveSupport.
func (b *Base) runCallbacks(body func() error) error {
	for _, f := range b.class.before {
		if !f.applies(b) {
			continue
		}
		if err := f.fn(b); err != nil {
			return err
		}
		if b.performed {
			return nil
		}
	}
	if err := b.runAround(0, body); err != nil {
		return err
	}
	for _, f := range b.class.after {
		if !f.applies(b) {
			continue
		}
		if err := f.fn(b); err != nil {
			return err
		}
	}
	return nil
}

// runAround recursively wraps body in the applicable around filters.
func (b *Base) runAround(i int, body func() error) error {
	for i < len(b.class.around) {
		f := b.class.around[i]
		if f.applies(b) {
			next := i + 1
			return f.around(b, func() error { return b.runAround(next, body) })
		}
		i++
	}
	return body()
}
