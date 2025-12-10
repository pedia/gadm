package gadm

import (
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlueprint(t *testing.T) {
	is := assert.New(t)

	is.Equal([]string{"a", "b"}, strings.SplitN("a.b", ".", 2))
	is.Equal([]string{"a", "b.c"}, strings.SplitN("a.b.c", ".", 2))
	is.Equal([]string{"", "b"}, strings.SplitN(".b", ".", 2))
	is.Equal([]string{"", "b.c"}, strings.SplitN(".b.c", ".", 2))
	is.Equal([]string{"a"}, strings.SplitN("a", ".", 2))

	is.NotEqual("a/", path.Join("a", "/"))

	h := func(http.ResponseWriter, *http.Request) {}

	f := &Blueprint{
		Name:     "Foo",
		Endpoint: "foo",
		Path:     "/foo",
		Children: map[string]*Blueprint{
			// flask-admin use `admin.index`, in `gadmin` should not use `view.index``
			"index":        {Handler: h, Endpoint: "index", Path: "/"},
			"index_view":   {Handler: h, Endpoint: "index_view", Path: "/"},
			"create_view":  {Handler: h, Endpoint: "create_view", Path: "/new"},
			"details_view": {Handler: h, Endpoint: "details_view", Path: "/details"},
			"action_view":  {Handler: h, Endpoint: "action_view", Path: "/action"},
			"execute_view": {Handler: h, Endpoint: "execute_view", Path: "/execute"},
			"edit_view":    {Handler: h, Endpoint: "edit_view", Path: "/edit"},
			"delete_view":  {Handler: h, Endpoint: "delete_view", Path: "/delete"},
			"export":       {Handler: h, Endpoint: "export", Path: "/export"},
			"static":       {Endpoint: "static", Path: "/static/", StaticFolder: "path/to/static"},
		},
	}

	// is.PanicsWithError("endpoint 'not' miss in `foo`", func() { f.GetUrl(".not") })
	is.NotPanics(func() { f.GetUrl(".not") })

	is.Equal("/foo/", must(f.GetUrl(".index")))
	is.Equal("/foo/", must(f.GetUrl("foo.index")))
	is.Equal("/foo/edit", must(f.GetUrl("foo.edit_view")))

	is.Equal("/foo/", must(f.GetUrl(".index")))
	is.Equal("/foo/?a=A", must(f.GetUrl(".index", "a", "A")))
	is.Equal("/foo/?a=B&a=C", must(f.GetUrl(".index", "a", "B", "a", "C")))
	is.Equal("/foo/static/?file=a.css", must(f.GetUrl(".static", "file", "a.css")))

	mux := http.NewServeMux()
	f.registerTo(mux, "/parent")

	a := Blueprint{
		Name:     "Admin",
		Endpoint: "admin",
		Path:     "/admin",
		Children: map[string]*Blueprint{
			"index": {Endpoint: "index", Path: "/"},
		},
	}
	is.Nil(f.Parent)
	a.AddChild(f)
	is.NotNil(f.Parent)

	is.Equal("/admin/", must(a.GetUrl(".index")))
	is.Equal("/admin/foo/", must(a.GetUrl("foo.index")))
	is.Equal("/admin/foo/edit", must(a.GetUrl("foo.edit_view")))

	is.Equal("/admin/foo/edit", must(f.GetUrl(".edit_view")))

	// f.AddChild(&Blueprint{Endpoint: "bar", Path: "/haha"})
	// is.Equal("/admin/foo/haha", must(a.GetUrl("foo.bar")))

	// level3, replace bar
	f.AddChild(&Blueprint{Endpoint: "bar", Path: "/bar", Children: map[string]*Blueprint{
		"index": {Endpoint: "index", Path: "/"},
	}})

	is.Equal("/admin/foo/bar/", must(a.GetUrl("foo.bar.index")))
	is.Equal("/admin/foo/bar/", must(f.GetUrl("foo.bar.index")))
}

func TestMenuAccess(t *testing.T) {
	is := assert.New(t)
	root := Menu{Path: "/admin/", Children: []*Menu{
		{Path: "/admin/article/", Roles: []string{"editor"}},
		{Path: "/admin/category/", Roles: []string{"user", "editor"}},
		{Path: "/admin/foo/"},
	}}
	is.True(root.hasAccess([]string{"user"}))
	is.True(root.hasAccess([]string{"editor"}))
	is.True(root.hasAccess(nil))
	is.False(root.Children[0].hasAccess([]string{"user"}))
	is.True(root.Children[0].hasAccess([]string{"editor"}))
	is.True(root.Children[1].hasAccess([]string{"user"}))
	is.True(root.Children[1].hasAccess([]string{"editor"}))
	is.True(root.Children[2].hasAccess([]string{"editor"}))

	d := root.dict("/admin/", []string{"user", "editor"})
	is.True(d["IsActive"].(bool))
	is.True(d["IsVisible"].(bool))

	d = root.Children[0].dict("/admin/", []string{"user", "editor"})
	is.False(d["IsActive"].(bool))
	is.True(d["IsVisible"].(bool))

	d = root.dict("/admin/", []string{"user"})
	is.True(d["IsActive"].(bool)) // weird
	is.True(d["IsVisible"].(bool))
}
