// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import (
	"testing"
)

// fullSet exercises the whole DSL surface used across the tests.
func fullSet(t *testing.T) *RouteSet {
	t.Helper()
	rs := NewRouteSet()
	err := rs.Draw(func(m *Mapper) {
		m.Root("home#index")
		m.Get("/about", To("pages#about"), As("about"))
		m.Match("/any", To("misc#any"), As("any"), Via("GET", "POST"))
		m.Post("/login", Controller("sessions"), Action("create"), As("login"))
		m.Put("/put", To("p#u"))
		m.Patch("/patch", To("p#p"))
		m.Delete("/del", To("d#x"))
		m.Match("/anyverb", To("misc#anyverb"), As("anyverb")) // no Via -> any method
		m.Get("/solo", To("solo"), As("solo"))                 // target without '#'
		m.Get("/health", Controller("monitors"), As("health")) // controller, default action
		m.Get("/files/*path", To("files#show"), As("file"))
		m.Get("/num/:id", To("n#s"), As("num"),
			Constraints(map[string]string{"id": `\d+`}),
			Defaults(map[string]string{"locale": "en"}))
		m.Resources("posts", func(m *Mapper) {
			m.Member(func(m *Mapper) { m.Get("preview") })
			m.Collection(func(m *Mapper) { m.Get("search") })
			m.Get("bare") // bare route inside resource -> member
			m.Get("promote", On("collection"), As("boost"))
			m.Get("archive", On("member"), To("posts#arch"), As("arch")) // member + to + as
			m.Resources("comments", nil, Only("index", "show"))
		})
		m.Resource("profile", func(m *Mapper) {
			m.Get("settings", On("member"))
		}, ResController("profiles"))
		m.Resources("photos", nil, Except("destroy"), PathName("pics"),
			ResController("images"), ResParam("photo_id"))
		m.Namespace("admin", func(m *Mapper) {
			m.Resources("users", nil)
			m.Root("dashboard#index", Defaults(map[string]string{"x": "1"}))
		})
		m.Scope(ScopeOpts{Path: "api", Module: "api", As: "api",
			Constraints: map[string]string{"id": `\d+`},
			Defaults:    map[string]string{"format": "json"}}, func(m *Mapper) {
			m.Get("/ping", To("health#ping"), As("ping"))
		})
		m.ConstraintsBlock(map[string]string{"id": `\d+`}, func(m *Mapper) {
			m.Get("/widget/:id", To("widgets#show"), As("widget"))
		})
	})
	if err != nil {
		t.Fatalf("draw error: %v", err)
	}
	return rs
}

func TestRecognize(t *testing.T) {
	rs := fullSet(t)
	cases := []struct {
		method, path       string
		controller, action string
		params             map[string]any
	}{
		{"GET", "/", "home", "index", nil},
		{"HEAD", "/", "home", "index", nil}, // HEAD routes as GET
		{"GET", "/about", "pages", "about", nil},
		{"GET", "/about.json", "pages", "about", map[string]any{"format": "json"}},
		{"POST", "/any", "misc", "any", nil},
		{"GET", "/any", "misc", "any", nil},
		{"POST", "/login", "sessions", "create", nil},
		{"PUT", "/put", "p", "u", nil},
		{"PATCH", "/patch", "p", "p", nil},
		{"DELETE", "/del", "d", "x", nil},
		{"PUT", "/anyverb", "misc", "anyverb", nil}, // match-any route
		{"GET", "/solo", "solo", "", nil},           // target without '#'
		{"GET", "/health", "monitors", "health", nil},
		{"GET", "/files/a/b/c.txt", "files", "show", map[string]any{"path": "a/b/c.txt"}},
		{"GET", "/num/12", "n", "s", map[string]any{"id": "12", "locale": "en"}},
		{"GET", "/posts", "posts", "index", nil},
		{"POST", "/posts", "posts", "create", nil},
		{"GET", "/posts/new", "posts", "new", nil},
		{"GET", "/posts/5/edit", "posts", "edit", map[string]any{"id": "5"}},
		{"GET", "/posts/5", "posts", "show", map[string]any{"id": "5"}},
		{"PATCH", "/posts/5", "posts", "update", map[string]any{"id": "5"}},
		{"PUT", "/posts/5", "posts", "update", map[string]any{"id": "5"}},
		{"DELETE", "/posts/5", "posts", "destroy", map[string]any{"id": "5"}},
		{"GET", "/posts/search", "posts", "search", nil},
		{"GET", "/posts/5/preview", "posts", "preview", map[string]any{"id": "5"}},
		{"GET", "/posts/5/bare", "posts", "bare", map[string]any{"id": "5"}},
		{"GET", "/posts/promote", "posts", "promote", nil},
		{"GET", "/posts/9/comments", "comments", "index", map[string]any{"post_id": "9"}},
		{"GET", "/posts/9/comments/3", "comments", "show", map[string]any{"post_id": "9", "id": "3"}},
		{"GET", "/profile", "profiles", "show", nil},
		{"GET", "/profile/new", "profiles", "new", nil},
		{"GET", "/profile/edit", "profiles", "edit", nil},
		{"POST", "/profile", "profiles", "create", nil},
		{"PATCH", "/profile", "profiles", "update", nil},
		{"DELETE", "/profile", "profiles", "destroy", nil},
		{"GET", "/profile/settings", "profiles", "settings", nil},
		{"GET", "/posts/5/archive", "posts", "arch", map[string]any{"id": "5"}},
		{"GET", "/admin", "admin/dashboard", "index", map[string]any{"x": "1"}},
		{"GET", "/pics", "images", "index", nil},
		{"GET", "/pics/7", "images", "show", map[string]any{"photo_id": "7"}},
		{"GET", "/admin/users/3", "admin/users", "show", map[string]any{"id": "3"}},
		{"GET", "/api/ping", "api/health", "ping", map[string]any{"format": "json"}},
		{"GET", "/widget/42", "widgets", "show", map[string]any{"id": "42"}},
	}
	for _, tc := range cases {
		rec, ok := rs.Recognize(tc.method, tc.path)
		if !ok {
			t.Errorf("%s %s: no match", tc.method, tc.path)
			continue
		}
		if rec.Controller != tc.controller || rec.Action != tc.action {
			t.Errorf("%s %s -> %s#%s, want %s#%s", tc.method, tc.path,
				rec.Controller, rec.Action, tc.controller, tc.action)
		}
		for k, v := range tc.params {
			if rec.Params[k] != v {
				t.Errorf("%s %s param %s=%v, want %v", tc.method, tc.path, k, rec.Params[k], v)
			}
		}
	}
}

func TestRecognizeFailures(t *testing.T) {
	rs := fullSet(t)
	// Photos except:destroy — DELETE must not match.
	if _, ok := rs.Recognize("DELETE", "/pics/1"); ok {
		t.Errorf("except:destroy still routes DELETE")
	}
	// Comments only index/show — no create.
	if _, ok := rs.Recognize("POST", "/posts/1/comments"); ok {
		t.Errorf("only:index,show still routes POST")
	}
	// Constraint \d+ rejects non-numeric.
	if _, ok := rs.Recognize("GET", "/num/xx"); ok {
		t.Errorf("constraint should reject /num/xx")
	}
	// Unknown path.
	if _, ok := rs.Recognize("GET", "/nowhere"); ok {
		t.Errorf("unexpected match for /nowhere")
	}
	// Wrong verb on a matching path.
	if _, ok := rs.Recognize("DELETE", "/about"); ok {
		t.Errorf("verb mismatch should not match")
	}
}

func TestPathHelpers(t *testing.T) {
	rs := fullSet(t)
	check := func(got string, err error, want string) {
		t.Helper()
		if err != nil {
			t.Fatalf("helper error: %v", err)
		}
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	p, err := rs.Path("posts", nil)
	check(p, err, "/posts")
	p, err = rs.Path("post", map[string]any{"id": 1})
	check(p, err, "/posts/1")
	p, err = rs.PathArgs("post", 2)
	check(p, err, "/posts/2")
	p, err = rs.Path("new_post", nil)
	check(p, err, "/posts/new")
	p, err = rs.Path("edit_post", map[string]any{"id": 7})
	check(p, err, "/posts/7/edit")
	p, err = rs.Path("preview_post", map[string]any{"id": 9})
	check(p, err, "/posts/9/preview")
	p, err = rs.Path("search_posts", nil)
	check(p, err, "/posts/search")
	p, err = rs.Path("boost_posts", nil)
	check(p, err, "/posts/promote")
	p, err = rs.Path("admin_users", nil)
	check(p, err, "/admin/users")
	p, err = rs.Path("admin_user", map[string]any{"id": 3})
	check(p, err, "/admin/users/3")
	p, err = rs.Path("post_comments", map[string]any{"post_id": 4})
	check(p, err, "/posts/4/comments")
	p, err = rs.PathArgs("file", "x/y/z")
	check(p, err, "/files/x/y/z")
	p, err = rs.Path("root", nil)
	check(p, err, "/")
	// Format + query params.
	p, err = rs.PathArgs("post", 2, map[string]any{"format": "json", "q": "x"})
	check(p, err, "/posts/2.json?q=x")
	// Multiple query keys are sorted.
	p, err = rs.Path("about", map[string]any{"b": "2", "a": "1"})
	check(p, err, "/about?a=1&b=2")
}

func TestUrlFor(t *testing.T) {
	rs := fullSet(t)
	p, err := rs.UrlFor(map[string]any{"controller": "posts", "action": "show", "id": 4})
	if err != nil || p != "/posts/4" {
		t.Fatalf("url_for show: %q %v", p, err)
	}
	p, err = rs.UrlFor(map[string]any{"controller": "posts", "action": "index"})
	if err != nil || p != "/posts" {
		t.Fatalf("url_for index: %q %v", p, err)
	}
	// No match.
	if _, err := rs.UrlFor(map[string]any{"controller": "nope", "action": "x"}); err == nil {
		t.Fatalf("url_for should fail for unknown target")
	}
	// Matches controller/action but required segment missing: skip that route,
	// eventually error since no other route matches.
	if _, err := rs.UrlFor(map[string]any{"controller": "posts", "action": "show"}); err == nil {
		t.Fatalf("url_for show without id should fail")
	}
}

func TestHelperErrors(t *testing.T) {
	rs := fullSet(t)
	if _, err := rs.Path("does_not_exist", nil); err == nil {
		t.Fatalf("Path unknown should error")
	} else if _, ok := err.(*RouteNotFound); !ok {
		t.Fatalf("wrong error type: %v", err)
	}
	if _, err := rs.PathArgs("does_not_exist"); err == nil {
		t.Fatalf("PathArgs unknown should error")
	}
	// Wrong positional arity.
	if _, err := rs.PathArgs("post"); err == nil {
		t.Fatalf("PathArgs arity should error")
	}
	// Missing required key for a named path.
	if _, err := rs.Path("post", nil); err == nil {
		t.Fatalf("missing id should error")
	} else if _, ok := err.(*MissingRouteKeys); !ok {
		t.Fatalf("wrong error type: %v", err)
	}
}

func TestDrawCompileError(t *testing.T) {
	rs := NewRouteSet()
	err := rs.Draw(func(m *Mapper) {
		m.Get("/bad/(:id", To("x#y")) // unbalanced group
		m.Get("/ok", To("a#b"))       // setErr keeps only the first error
	})
	if err == nil {
		t.Fatalf("expected compile error")
	}
}

func TestRoutesCopy(t *testing.T) {
	rs := fullSet(t)
	routes := rs.Routes()
	n := len(routes)
	routes[0] = nil
	if len(rs.Routes()) != n || rs.Routes()[0] == nil {
		t.Fatalf("Routes should return a copy")
	}
}

func TestMemberOutsideResourceIsNoop(t *testing.T) {
	rs := NewRouteSet()
	err := rs.Draw(func(m *Mapper) {
		m.Member(func(m *Mapper) { m.Get("x", To("a#b")) }) // no enclosing resource
		m.Collection(func(m *Mapper) { m.Get("y", To("a#c")) })
	})
	if err != nil {
		t.Fatalf("draw: %v", err)
	}
	if len(rs.Routes()) != 0 {
		t.Fatalf("member/collection outside a resource should add nothing")
	}
}

func TestMemberPathHelpers(t *testing.T) {
	rs := fullSet(t)
	p, err := rs.Path("arch_post", map[string]any{"id": 3})
	if err != nil || p != "/posts/3/archive" {
		t.Fatalf("arch_post: %q %v", p, err)
	}
	p, err = rs.Path("settings_profile", nil)
	if err != nil || p != "/profile/settings" {
		t.Fatalf("settings_profile: %q %v", p, err)
	}
}

func TestUrlForNonStringController(t *testing.T) {
	rs := fullSet(t)
	// A non-string controller coerces to "" and matches nothing.
	if _, err := rs.UrlFor(map[string]any{"controller": 123, "action": "show"}); err == nil {
		t.Fatalf("expected no match for non-string controller")
	}
}

func TestAppendQueryDropsBlank(t *testing.T) {
	rs := fullSet(t)
	// A nil extra param yields no query string.
	p, err := rs.Path("posts", map[string]any{"blank": nil})
	if err != nil || p != "/posts" {
		t.Fatalf("blank query param: %q %v", p, err)
	}
}

func TestGlobMissingKey(t *testing.T) {
	rs := fullSet(t)
	if _, err := rs.Path("file", nil); err == nil {
		t.Fatalf("missing glob key should error")
	}
}

func TestErrorMessages(t *testing.T) {
	if (&RouteNotFound{Name: "x"}).Error() == "" {
		t.Fatalf("RouteNotFound message")
	}
	if (&MissingRouteKeys{Spec: "/x"}).Error() == "" {
		t.Fatalf("MissingRouteKeys message")
	}
	if (&UnroutableParameters{Controller: "a", Action: "b"}).Error() == "" {
		t.Fatalf("UnroutableParameters message")
	}
}

func TestNodeMarkers(t *testing.T) {
	// The isNode marker methods exist only to seal the node interface.
	litNode{}.isNode()
	symNode{}.isNode()
	starNode{}.isNode()
	groupNode{}.isNode()
}

func TestToParam(t *testing.T) {
	cases := []struct {
		in   any
		want string
		ok   bool
	}{
		{nil, "", false},
		{"", "", false},
		{"x", "x", true},
		{true, "true", true},
		{7, "7", true},
		{int64(9), "9", true},
		{3.5, "3.5", true},
		{stringer(""), "", false},
		{stringer("s"), "s", true},
	}
	for _, c := range cases {
		got, ok := toParam(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("toParam(%v) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

type stringer string

func (s stringer) String() string { return string(s) }

func TestCompilePatternErrors(t *testing.T) {
	for _, spec := range []string{
		"/x(", // unbalanced '('
		"/x)", // unexpected ')'
		"/:",  // empty dynamic name
		"/*",  // empty glob name
	} {
		if _, err := compilePattern(spec, nil); err == nil {
			t.Errorf("compilePattern(%q) should error", spec)
		}
	}
	// Invalid requirement regexp.
	if _, err := compilePattern("/x/:id", map[string]string{"id": "("}); err == nil {
		t.Errorf("bad requirement should fail to compile")
	}
}

func TestStarRequirementAndOptional(t *testing.T) {
	// Glob with a custom requirement, plus an optional group that participates
	// and one that does not.
	pat, err := compilePattern("/f/*path(.:format)", map[string]string{"path": `[a-z/]+`})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m, ok := pat.match("/f/a/b.json")
	if !ok || m["path"] != "a/b" || m["format"] != "json" {
		t.Fatalf("match with format: %v %v", m, ok)
	}
	m, ok = pat.match("/f/a/b")
	if !ok || m["path"] != "a/b" {
		t.Fatalf("match without format: %v %v", m, ok)
	}
	if _, ok := m["format"]; ok {
		t.Fatalf("optional format should be absent")
	}
	if _, ok := pat.match("/f/ABC"); ok {
		t.Fatalf("requirement should reject uppercase")
	}
	// Generation of an optional group that is skipped when its symbol is absent.
	out, gok := pat.generate(map[string]any{"path": "x/y"}, map[string]bool{})
	if !gok || out != "/f/x/y" {
		t.Fatalf("generate skip optional: %q %v", out, gok)
	}
}
