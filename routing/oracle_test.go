// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// oracleScript builds a representative ActionDispatch route set in MRI and dumps
// its recognition and path-helper results as JSON. The Go route set below
// mirrors this draw exactly; the test asserts the two agree, pinning routing
// fidelity to the real actionpack gem. It skips when ruby or the gem is absent.
const oracleScript = `
require 'action_dispatch'
require 'json'

routes = ActionDispatch::Routing::RouteSet.new
routes.draw do
  root to: 'home#index'
  get '/about', to: 'pages#about', as: 'about'
  get '/files/*path', to: 'files#show', as: 'file'
  get '/num/:id', to: 'n#s', as: 'num', constraints: { id: /\d+/ }
  resources :posts do
    member { get 'preview' }
    collection { get 'search' }
  end
  namespace :admin do
    resources :users
  end
end

probes = [
  ['GET','/'], ['GET','/about'], ['GET','/about.json'],
  ['GET','/files/a/b/c'], ['GET','/num/12'],
  ['GET','/posts'], ['POST','/posts'], ['GET','/posts/new'],
  ['GET','/posts/5'], ['GET','/posts/5/edit'], ['PATCH','/posts/5'],
  ['DELETE','/posts/5'], ['GET','/posts/search'], ['GET','/posts/5/preview'],
  ['GET','/admin/users/3']
]

recog = probes.map do |meth, path|
  begin
    r = routes.recognize_path(path, method: meth)
    params = r.reject { |k, _| [:controller, :action].include?(k) }
                .transform_keys(&:to_s).transform_values(&:to_s)
    { 'ok' => true, 'controller' => r[:controller], 'action' => r[:action], 'params' => params }
  rescue ActionController::RoutingError
    { 'ok' => false, 'params' => {} }
  end
end

h = routes.url_helpers
helpers = {
  'posts'        => h.posts_path,
  'post'         => h.post_path(1),
  'new_post'     => h.new_post_path,
  'edit_post'    => h.edit_post_path(7),
  'preview_post' => h.preview_post_path(9),
  'search_posts' => h.search_posts_path,
  'admin_users'  => h.admin_users_path,
  'admin_user'   => h.admin_user_path(3),
  'about'        => h.about_path,
  'file'         => h.file_path('a/b/c'),
}
puts JSON.generate({ 'recog' => recog, 'helpers' => helpers, 'probes' => probes })
`

type oracleOut struct {
	Recog []struct {
		OK         bool              `json:"ok"`
		Controller string            `json:"controller"`
		Action     string            `json:"action"`
		Params     map[string]string `json:"params"`
	} `json:"recog"`
	Helpers map[string]string `json:"helpers"`
	Probes  [][]string        `json:"probes"`
}

// oracleRoutes mirrors the MRI draw in oracleScript using this package's DSL.
func oracleRoutes(t *testing.T) *RouteSet {
	t.Helper()
	rs := NewRouteSet()
	err := rs.Draw(func(m *Mapper) {
		m.Root("home#index")
		m.Get("/about", To("pages#about"), As("about"))
		m.Get("/files/*path", To("files#show"), As("file"))
		m.Get("/num/:id", To("n#s"), As("num"), Constraints(map[string]string{"id": `\d+`}))
		m.Resources("posts", func(m *Mapper) {
			m.Member(func(m *Mapper) { m.Get("preview") })
			m.Collection(func(m *Mapper) { m.Get("search") })
		})
		m.Namespace("admin", func(m *Mapper) {
			m.Resources("users", nil)
		})
	})
	if err != nil {
		t.Fatalf("draw: %v", err)
	}
	return rs
}

func TestMRIRoutingOracle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("oracle uses a POSIX ruby invocation")
	}
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not installed")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "oracle.rb")
	if err := os.WriteFile(script, []byte(oracleScript), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	out, err := exec.Command("ruby", script).Output()
	if err != nil {
		t.Skipf("actionpack oracle unavailable (%v)", err)
	}
	var oracle oracleOut
	if err := json.Unmarshal(out, &oracle); err != nil {
		t.Fatalf("decode oracle: %v\n%s", err, out)
	}

	rs := oracleRoutes(t)

	// Recognition parity.
	for i, probe := range oracle.Probes {
		method, path := probe[0], probe[1]
		exp := oracle.Recog[i]
		rec, ok := rs.Recognize(method, path)
		if ok != exp.OK {
			t.Errorf("%s %s: recognized=%v, MRI=%v", method, path, ok, exp.OK)
			continue
		}
		if !ok {
			continue
		}
		if rec.Controller != exp.Controller || rec.Action != exp.Action {
			t.Errorf("%s %s -> %s#%s, MRI %s#%s", method, path,
				rec.Controller, rec.Action, exp.Controller, exp.Action)
		}
		for k, v := range exp.Params {
			if got, _ := rec.Params[k].(string); got != v {
				t.Errorf("%s %s param %s=%q, MRI %q", method, path, k, got, v)
			}
		}
		// No extra dynamic params beyond MRI's (ignoring controller/action).
		for k, gv := range rec.Params {
			if k == "controller" || k == "action" {
				continue
			}
			if _, ok := exp.Params[k]; !ok {
				t.Errorf("%s %s produced extra param %s=%v not in MRI", method, path, k, gv)
			}
		}
	}

	// Path-helper parity.
	helperArgs := map[string]func() (string, error){
		"posts":        func() (string, error) { return rs.Path("posts", nil) },
		"post":         func() (string, error) { return rs.PathArgs("post", 1) },
		"new_post":     func() (string, error) { return rs.Path("new_post", nil) },
		"edit_post":    func() (string, error) { return rs.PathArgs("edit_post", 7) },
		"preview_post": func() (string, error) { return rs.PathArgs("preview_post", 9) },
		"search_posts": func() (string, error) { return rs.Path("search_posts", nil) },
		"admin_users":  func() (string, error) { return rs.Path("admin_users", nil) },
		"admin_user":   func() (string, error) { return rs.PathArgs("admin_user", 3) },
		"about":        func() (string, error) { return rs.Path("about", nil) },
		"file":         func() (string, error) { return rs.PathArgs("file", "a/b/c") },
	}
	for name, want := range oracle.Helpers {
		fn, ok := helperArgs[name]
		if !ok {
			t.Fatalf("no Go helper for %q", name)
		}
		got, err := fn()
		if err != nil {
			t.Errorf("%s_path: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%s_path = %q, MRI %q", name, got, want)
		}
	}
}
