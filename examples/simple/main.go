package main

import (
	"gadm"
	"net/http"
)

type MyView struct {
	*gadm.BaseView
}

func NewMyView() *MyView {
	v := &MyView{
		BaseView: gadm.NewView(gadm.Menu{Name: "View3"}),
	}
	v.Expose("/", v.indexHandler)
	return v
}

func (M *MyView) indexHandler(w http.ResponseWriter, r *http.Request) {
	M.Render(w, r, "examples/simple/templates/myadmin.gotmpl", nil, nil)
}

func main() {
	admin := gadm.NewAdmin("Example: Simple Views")

	v := gadm.NewView(gadm.Menu{Name: "View1"})
	v.Expose("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("raw view"))
	})
	admin.AddView(v)
	admin.AddView(gadm.NewView(gadm.Menu{Name: "View2"}))
	admin.AddView(NewMyView())
	admin.Run()
}
