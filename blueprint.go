package gadm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gadm/isdebug"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/samber/lo"
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
	if child.Path != "" && !strings.HasPrefix(child.Path, "/") {
		fmt.Printf("Blueprint(%s) Path: %s not valid", child.Endpoint, child.Path)
	}
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

		if isdebug.On {
			log.Printf("%s handle %s", b.Name, parent+b.Path)
		}
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
	if eps[0] == "" || eps[0] == b.Endpoint || inmap(b.Children, eps[0]) {
		i := 1
		if inmap(b.Children, eps[0]) {
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

func (b *Blueprint) MarshalJSON() ([]byte, error) {
	w := bytes.NewBuffer(nil)
	err := json.NewEncoder(w).Encode(map[string]any{
		"endpoint":        b.Endpoint,
		"path":            b.Path,
		"name":            b.Name,
		"children":        b.Children,
		"static_folder":   b.StaticFolder,
		"template_folder": b.TemplateFolder,
	})
	return w.Bytes(), err
}

// Tree liked structure
type Menu struct {
	Name  string
	Path  string
	Icon  string
	Class string
	Roles []string

	Children []*Menu
}

func (M *Menu) AddMenu(i *Menu, category ...string) {
	parent := M.find(firstOr(category, M.Name))
	if parent == nil {
		parent = &Menu{Name: firstOr(category)}
		M.Children = append(M.Children, parent)
	}
	parent.Children = append(parent.Children, i)
}

func (M *Menu) find(name string) *Menu {
	if M.Name == name {
		return M
	}

	c, _ := lo.Find(M.Children, func(m *Menu) bool {
		return m.Name == name
	})
	return c
}

func (M *Menu) dict(current_path string, user_roles []string) map[string]any {
	return map[string]any{
		"Name":         M.Name,
		"Path":         M.Path,
		"Icon ":        M.Icon,
		"Class":        M.Class,
		"IsActive":     M.Path != "" && (current_path == M.Path || strings.HasPrefix(current_path, M.Path)),
		"IsVisible":    M.hasAccess(user_roles),
		"IsAccessible": M.hasAccess(user_roles),
		"Children": lo.Map(M.Children, func(child *Menu, _ int) map[string]any {
			return child.dict(current_path, user_roles)
		}),
	}
}

func (M *Menu) hasAccess(user_roles []string) bool {
	if len(M.Roles) == 0 {
		return true
	}

	for _, role := range user_roles {
		if role == "admin" {
			return true
		}

		if slices.Contains(M.Roles, role) {
			return true
		}
	}

	// check children
	for _, child := range M.Children {
		if child.hasAccess(user_roles) {
			return true
		}
	}
	return false
}
