package gadm

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Category string // tree liked
	Name     string
	Path     string

	Icon  string
	Class string

	IsActive     bool // TODO:
	IsVisible    bool
	IsAccessible bool

	Children []*Menu
}

// TODO: AddCategory/AddLink/AddMenuItem
func (M *Menu) AddMenu(i *Menu, category ...string) {
	if i.Category == "" {
		if i.Path == "" && !strings.HasPrefix(i.Path, "/") {
			np := "/" + strings.ToLower(i.Name)
			log.Printf(`menu(%s) path '%s' invalid, fixed to '%s'`, i.Name, i.Path, np)
			i.Path = np
		}
	}

	parent := M.find(firstOr(category, ""))
	if parent != nil {
		parent.Children = append(parent.Children, i)
	} else {
		// stub, create a new stub or self is stub
		stub := &Menu{Name: i.Category, Category: i.Category}
		stub.Children = append(stub.Children, i)
		M.Children = append(M.Children, stub)
	}
}

func (M *Menu) find(cate string) *Menu {
	if M.Category == cate {
		return M
	}

	c, _ := lo.Find(M.Children, func(m *Menu) bool {
		return m.Category == cate
	})
	return c
}
