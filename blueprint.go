package gadm

import (
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
)

// like flask.Blueprint
//
// | Name  | Endpoint  | Path       |
// |-------|-----------|------------|
// | Foo   | foo       | /foo/      |
// |       | .index    | /          |
// |       | .action   | /action    |
// |       | foo.index | /foo/      |
// | Admin | admin     | /admin/    |
// |       | .index    | /          |
//
// A blueprint is A model and dependent pages
type Blueprint struct {
	Endpoint string // {foo}.index
	Path     string // /foo
	Name     string // Foo
	Children map[string]*Blueprint
	Parent   *Blueprint

	Handler http.HandlerFunc

	// Custom register into http.ServerMux
	RegisterFunc func(*http.ServeMux, string, *Blueprint)

	StaticFolder   string
	TemplateFolder string

	// StaticUrlPath
	// ErrorHandler
}

// like flask `Blueprint.Register`
func (b *Blueprint) AddChild(child *Blueprint) (err error) {
	if b.Children == nil {
		b.Children = map[string]*Blueprint{}
	}
	if _, ok := b.Children[child.Endpoint]; ok {
		// allow replace?
		err = fmt.Errorf("parent %s duplicated child %s", b.Endpoint, child.Endpoint)
	} else {
		b.Children[child.Endpoint] = child

		child.Parent = b

		// fix all children's Parent
		fixPointer(b)
	}
	return err
}

func fixPointer(b *Blueprint) {
	for _, c := range b.Children {
		if c.Parent == nil {
			c.Parent = b
		}
		fixPointer(c)
	}
}

// Register all Blueprint to `http.ServeMux`
func (b *Blueprint) registerTo(mux *http.ServeMux, parent string) {
	if b.RegisterFunc != nil {
		b.RegisterFunc(mux, parent, b)
	} else if b.Handler != nil {
		if !strings.HasPrefix(b.Path, "/") {
			log.Printf("warning: Blueprint(%s path: %s) not start with /", b.Name, b.Path)
		}

		// log.Printf("%s handle %s", b.Name, parent+b.Path)
		if strings.HasSuffix(b.Path, "/") {
			mux.HandleFunc(parent+b.Path+"{$}", b.Handler)
		} else {
			mux.HandleFunc(parent+b.Path, b.Handler)
		}
	} else if b.Endpoint == "static" && b.StaticFolder != "" {
		// log.Printf("%s handle %s fs: %s", b.Name, parent+b.Path, b.StaticFolder)

		if !strings.HasSuffix(b.Path, "/") {
			panic("Blueprint(Name='static').Path should end with /")
		}

		fs := http.FileServer(http.Dir(b.StaticFolder))
		mux.Handle(parent+b.Path, // minified.Middleware(
			http.StripPrefix(parent+b.Path, fs))

		// TODO: add an endpoint
	}

	// Avoid `ServerMux` duplicated `Path`
	// eg: `index` `index_view` have same `path`
	up := map[string]bool{}
	for _, child := range b.Children {
		if unique := up[child.Path]; !unique {
			child.registerTo(mux, parent+b.Path)

			up[child.Path] = true
		}
	}
}

func (b *Blueprint) prefixOf(tail string) string {
	arr := []string{}
	for c := b.Parent; c != nil; c = c.Parent {
		arr = append(arr, c.Path)
	}

	slices.Reverse(arr)
	arr = append(arr, tail)
	return strings.Join(arr, "")
}

// endpoint arg like:
// {ep}
// {ep}.index
// {ep}.child.index
// child.index
func (b *Blueprint) GetUrl(endpoint string, qs ...any) (string, error) {
	eps := strings.Split(endpoint, ".")
	if eps[0] == "" || eps[0] == b.Endpoint || mapContains(b.Children, eps[0]) {
		i := 1
		if mapContains(b.Children, eps[0]) {
			i = 0
		}

		pa := []string{b.prefixOf(b.Path)}
		match := true
		curb := b
		for ; i < len(eps); i++ {
			child, ok := curb.Children[eps[i]]
			if !ok {
				match = false
				break
			}
			pa = append(pa, child.Path)
			curb = child
		}

		if match {
			res := strings.Join(pa, "")

			if len(qs) > 0 {
				res += "?" + pairsToQuery(qs...).Encode()
			}
			return res, nil
		}
	}
	if b.Parent != nil && !strings.HasPrefix(endpoint, ".") {
		return b.Parent.GetUrl(endpoint, qs...)
	}
	return "", fmt.Errorf(`endpoint miss for '%s'`, endpoint)
}

func (b *Blueprint) dict() map[string]any {
	o := map[string]any{
		"endpoint": b.Endpoint,
		"path":     b.Path,
		"handler":  b.Handler != nil,
		"name":     b.Name,
		"parent":   b.Parent != nil,
	}

	if b.Children != nil {
		oc := map[string]any{}
		for k, v := range b.Children {
			oc[k] = v.dict()
		}
		o["children"] = oc
	}
	return o
}

// menu

// admin scope:
// admin.logout_view
// admin.index
// admin.static

// security(login) scope:
// security.login /login
// security.logout /logout
// security.register /register
// security.forgot_password
// security.send_confirmation
