// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package controller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-ruby-actionpack/actionpack/dispatch"
	"github.com/go-ruby-actionpack/actionpack/parameters"
	"github.com/go-ruby-rack/rack"
)

func newBase(cl *Class) *Base {
	req := dispatch.NewRequest(rack.Env{})
	return cl.New(req, dispatch.NewResponse(), parameters.New(map[string]any{"id": "1"}))
}

func TestActionSeam(t *testing.T) {
	cl := NewClass("posts").
		Action("show", func(c *Base) error { return c.RenderPlain("shown") })
	b := newBase(cl)
	if err := b.Process("show"); err != nil {
		t.Fatalf("process: %v", err)
	}
	if !b.Performed() || b.Response().BodyString() != "shown" {
		t.Fatalf("render seam: performed=%v body=%q", b.Performed(), b.Response().BodyString())
	}
	if b.Rendered == nil || b.Rendered.Plain != "shown" {
		t.Fatalf("Rendered not recorded")
	}
	if b.Action() != "show" || b.Class() != cl {
		t.Fatalf("accessors")
	}
}

func TestRunActionSeam(t *testing.T) {
	var gotAction string
	cl := NewClass("posts").RunAction(func(c *Base, action string) error {
		gotAction = action
		return c.RenderPlain("via runAction:" + c.Action())
	})
	b := newBase(cl)
	if err := b.Process("whatever"); err != nil {
		t.Fatalf("process: %v", err)
	}
	if gotAction != "whatever" || !strings.Contains(b.Response().BodyString(), "whatever") {
		t.Fatalf("runAction seam: %q %q", gotAction, b.Response().BodyString())
	}
}

func TestActionNotFound(t *testing.T) {
	cl := NewClass("posts")
	b := newBase(cl)
	err := b.Process("missing")
	var anf *ActionNotFound
	if !errors.As(err, &anf) || anf.Action != "missing" || anf.Controller != "posts" {
		t.Fatalf("expected ActionNotFound, got %v", err)
	}
	if anf.Error() == "" {
		t.Fatalf("ActionNotFound message")
	}
}

func TestNewDefaults(t *testing.T) {
	cl := NewClass("x")
	b := cl.New(nil, nil, nil)
	if b.Response() == nil || b.Params() == nil || b.Request() != nil {
		t.Fatalf("New defaults: resp/params must be non-nil, request stays nil")
	}
	p := parameters.New(map[string]any{"a": "1"})
	b.SetParams(p)
	if b.Params() != p {
		t.Fatalf("SetParams")
	}
}

func TestFilterOrderAndConditions(t *testing.T) {
	var trace []string
	cl := NewClass("t").
		BeforeAction(func(c *Base) error { trace = append(trace, "before-all"); return nil }).
		BeforeAction(func(c *Base) error { trace = append(trace, "before-show-only"); return nil }, Only("show")).
		BeforeAction(func(c *Base) error { trace = append(trace, "before-not-edit"); return nil }, Except("edit")).
		BeforeAction(func(c *Base) error { trace = append(trace, "before-if"); return nil }, If(func(c *Base) bool { return c.Action() == "show" })).
		BeforeAction(func(c *Base) error { trace = append(trace, "before-unless"); return nil }, Unless(func(c *Base) bool { return c.Action() == "show" })).
		AroundAction(func(c *Base, yield func() error) error {
			trace = append(trace, "around-in")
			err := yield()
			trace = append(trace, "around-out")
			return err
		}).
		AfterAction(func(c *Base) error { trace = append(trace, "after"); return nil }).
		Action("show", func(c *Base) error { trace = append(trace, "action"); return nil })

	trace = nil
	newBase(cl).Process("show")
	want := []string{"before-all", "before-show-only", "before-not-edit", "before-if", "around-in", "action", "around-out", "after"}
	if strings.Join(trace, ",") != strings.Join(want, ",") {
		t.Fatalf("show trace:\n got %v\nwant %v", trace, want)
	}

	trace = nil
	// edit has no registered action: the ActionNotFound error propagates out of
	// the around stack, so after filters are skipped (as when an action raises).
	newBase(cl).Process("edit")
	want = []string{"before-all", "before-unless", "around-in", "around-out"}
	if strings.Join(trace, ",") != strings.Join(want, ",") {
		t.Fatalf("edit trace:\n got %v\nwant %v", trace, want)
	}
}

func TestBeforeFilterHalts(t *testing.T) {
	var trace []string
	cl := NewClass("t").
		BeforeAction(func(c *Base) error {
			trace = append(trace, "guard")
			return c.RedirectTo("/login")
		}).
		AfterAction(func(c *Base) error { trace = append(trace, "after"); return nil }).
		Action("secret", func(c *Base) error { trace = append(trace, "action"); return nil })
	b := newBase(cl)
	if err := b.Process("secret"); err != nil {
		t.Fatalf("process: %v", err)
	}
	if strings.Join(trace, ",") != "guard" {
		t.Fatalf("halt should skip action and after: %v", trace)
	}
	if b.RedirectedTo != "/login" || b.Response().Status() != 302 {
		t.Fatalf("redirect: %q %d", b.RedirectedTo, b.Response().Status())
	}
}

func TestAroundSkipAndFilterErrors(t *testing.T) {
	// Around filter that does not yield skips the action.
	ran := false
	cl := NewClass("t").
		AroundAction(func(c *Base, yield func() error) error { return nil }).
		Action("a", func(c *Base) error { ran = true; return nil })
	newBase(cl).Process("a")
	if ran {
		t.Fatalf("action should be skipped when around does not yield")
	}

	// Around filter skipped by Only condition; action still runs.
	ran = false
	cl2 := NewClass("t").
		AroundAction(func(c *Base, yield func() error) error { return yield() }, Only("other")).
		Action("a", func(c *Base) error { ran = true; return nil })
	newBase(cl2).Process("a")
	if !ran {
		t.Fatalf("action should run when around filter does not apply")
	}

	// Before filter returning an error.
	berr := errors.New("before-fail")
	cl3 := NewClass("t").BeforeAction(func(c *Base) error { return berr })
	if err := newBase(cl3).Process("a"); !errors.Is(err, berr) {
		t.Fatalf("before error: %v", err)
	}

	// After filter skipped by a condition (does not apply), then one that errors.
	aerr := errors.New("after-fail")
	afterRan := false
	cl4 := NewClass("t").
		AfterAction(func(c *Base) error { afterRan = true; return nil }, Only("other")).
		AfterAction(func(c *Base) error { return aerr }).
		Action("a", func(c *Base) error { return nil })
	if err := newBase(cl4).Process("a"); !errors.Is(err, aerr) {
		t.Fatalf("after error: %v", err)
	}
	if afterRan {
		t.Fatalf("conditional after filter should be skipped")
	}
}

func TestRescueFrom(t *testing.T) {
	// RescueType matches by concrete type.
	cl := NewClass("t").Action("boom", func(c *Base) error {
		return &parameters.ParameterMissing{Key: "user"}
	})
	RescueType[*parameters.ParameterMissing](cl, func(c *Base, err error) error {
		return c.Head(400)
	})
	b := newBase(cl)
	if err := b.Process("boom"); err != nil {
		t.Fatalf("rescued error should be nil: %v", err)
	}
	if b.Response().Status() != 400 {
		t.Fatalf("rescue handler status: %d", b.Response().Status())
	}

	// RescueFromIs matches wrapped sentinels; last-declared wins.
	sentinel := errors.New("nope")
	cl2 := NewClass("t").
		RescueFromIs(sentinel, func(c *Base, err error) error { return errors.New("first") }).
		RescueFromIs(sentinel, func(c *Base, err error) error { return c.Head(410) }).
		Action("x", func(c *Base) error { return fmt.Errorf("wrap: %w", sentinel) })
	b2 := newBase(cl2)
	if err := b2.Process("x"); err != nil {
		t.Fatalf("last rescuer should win: %v", err)
	}
	if b2.Response().Status() != 410 {
		t.Fatalf("last-wins status: %d", b2.Response().Status())
	}

	// Unhandled error passes through.
	cl3 := NewClass("t").
		RescueFrom(func(err error) bool { return false }, func(c *Base, err error) error { return nil }).
		Action("x", func(c *Base) error { return errors.New("unhandled") })
	if err := newBase(cl3).Process("x"); err == nil || err.Error() != "unhandled" {
		t.Fatalf("unhandled error should pass through: %v", err)
	}
}

func TestRescuePanic(t *testing.T) {
	// A panicked error value is routed through rescue_from.
	cl := NewClass("t").Action("p", func(c *Base) error {
		panic(&parameters.UnpermittedParameters{Keys: []string{"x"}})
	})
	RescueType[*parameters.UnpermittedParameters](cl, func(c *Base, err error) error {
		return c.Head(422)
	})
	b := newBase(cl)
	if err := b.Process("p"); err != nil {
		t.Fatalf("panic should be rescued: %v", err)
	}
	if b.Response().Status() != 422 {
		t.Fatalf("panic rescue status: %d", b.Response().Status())
	}

	// A non-error panic is re-raised.
	cl2 := NewClass("t").Action("p", func(c *Base) error { panic("boom-string") })
	func() {
		defer func() {
			if r := recover(); r != "boom-string" {
				t.Fatalf("non-error panic should re-raise, got %v", r)
			}
		}()
		newBase(cl2).Process("p")
	}()
}

func TestRenderVariants(t *testing.T) {
	// Renderer seam success.
	cl := NewClass("t").
		SetRenderer(func(c *Base, opts RenderOptions) (string, error) {
			return "rendered:" + opts.Template, nil
		}).
		Action("show", func(c *Base) error {
			return c.Render(RenderOptions{Template: "posts/show", Status: 201, ContentType: "text/html"})
		})
	b := newBase(cl)
	if err := b.Process("show"); err != nil {
		t.Fatalf("render: %v", err)
	}
	if b.Response().BodyString() != "rendered:posts/show" || b.Response().Status() != 201 {
		t.Fatalf("renderer output: %q %d", b.Response().BodyString(), b.Response().Status())
	}

	// Renderer seam error.
	rerr := errors.New("render-fail")
	cl2 := NewClass("t").
		SetRenderer(func(c *Base, opts RenderOptions) (string, error) { return "", rerr }).
		Action("show", func(c *Base) error { return c.Render(RenderOptions{}) })
	if err := newBase(cl2).Process("show"); !errors.Is(err, rerr) {
		t.Fatalf("renderer error should surface: %v", err)
	}

	// No renderer: plain fallback, default status 200.
	cl3 := NewClass("t").Action("show", func(c *Base) error {
		return c.Render(RenderOptions{Plain: "just text"})
	})
	b3 := newBase(cl3)
	b3.Process("show")
	if b3.Response().BodyString() != "just text" || b3.Response().Status() != 200 {
		t.Fatalf("plain fallback: %q %d", b3.Response().BodyString(), b3.Response().Status())
	}
}

func TestDoubleRender(t *testing.T) {
	cl := NewClass("t")
	check := func(name string, fn func(b *Base) error) {
		b := newBase(cl)
		if err := b.RenderPlain("first"); err != nil {
			t.Fatalf("%s first render: %v", name, err)
		}
		err := fn(b)
		var dre *DoubleRenderError
		if !errors.As(err, &dre) {
			t.Fatalf("%s: expected DoubleRenderError, got %v", name, err)
		}
		if dre.Error() == "" {
			t.Fatalf("DoubleRenderError message")
		}
	}
	check("render", func(b *Base) error { return b.Render(RenderOptions{Plain: "x"}) })
	check("redirect", func(b *Base) error { return b.RedirectTo("/x") })
	check("head", func(b *Base) error { return b.Head(200) })
}

func TestRedirectAndHead(t *testing.T) {
	cl := NewClass("t")
	b := newBase(cl)
	if err := b.RedirectTo("/here"); err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if b.Response().Status() != 302 || b.RedirectedTo != "/here" {
		t.Fatalf("default redirect: %d %q", b.Response().Status(), b.RedirectedTo)
	}
	b2 := newBase(cl)
	if err := b2.RedirectTo("/perm", 301); err != nil {
		t.Fatalf("redirect custom: %v", err)
	}
	if b2.Response().Status() != 301 {
		t.Fatalf("custom redirect status: %d", b2.Response().Status())
	}
	b3 := newBase(cl)
	if err := b3.Head(204); err != nil {
		t.Fatalf("head: %v", err)
	}
	if b3.Response().Status() != 204 || !b3.Performed() {
		t.Fatalf("head: %d %v", b3.Response().Status(), b3.Performed())
	}
}
