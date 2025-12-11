package gadm

import (
	"bytes"
	"html/template"
	"net/url"
	"os"

	"github.com/samber/lo"
	"github.com/spf13/cast"
)

type Query struct {
	// 0 based with omit
	Page     int `form:"page,omitempty"`
	PageSize int `form:"page_size,omitempty"`
	// column index: 0,1,... maybe `null.String` is better
	Sort string `form:"sort,omitempty"`
	// asc[default] or desc
	Desc   bool   `form:"desc,omitempty"`
	Search string `form:"search,omitempty"`

	// flt0_35=2024-10-28&flt2_27=Harry&flt3_0=1
	filters []*InputFilter
	args    []string
	base    string // base path

	default_page_size int
}

func DefaultQuery() *Query {
	return &Query{
		Page:              0,
		PageSize:          20,
		default_page_size: 20,
	}
}

func (q *Query) withArgs(args ...any) *Query {
	if q.args == nil {
		q.args = pyslice(args...)
	} else {
		q.args = append(q.args, pyslice(args)...)
	}
	return q
}

func (q *Query) Get(arg string) string {
	for i := 0; i < len(q.args); i += 2 {
		if q.args[i] == arg {
			return q.args[i+1]
		}
	}
	return ""
}

func (q *Query) toValues() url.Values {
	uv := url.Values{}
	if q.Page > 0 {
		uv.Set("page", cast.ToString(q.Page))
	}
	if q.default_page_size != q.PageSize {
		uv.Set("page_size", cast.ToString(q.PageSize))
	}
	if q.Sort != "" {
		uv.Set("sort", q.Sort)
	}
	if q.Desc {
		uv.Set("desc", "1") // encode bool to 1
	}
	if q.Search != "" {
		uv.Set("search", q.Search)
	}

	for i := 0; i < len(q.args); i += 2 {
		uv.Add(q.args[i], q.args[i+1])
	}
	return uv
}

func (q *Query) urlForPage(page int) string {
	nq := Query{
		Page:              page, //
		PageSize:          q.PageSize,
		Sort:              q.Sort,
		Desc:              q.Desc,
		Search:            q.Search,
		default_page_size: q.default_page_size,
		args:              q.args,
	}

	uv := nq.toValues()
	if len(uv) > 0 {
		return q.base + "?" + uv.Encode()
	}
	return q.base
}

// generate pager or json
type Result struct {
	*Query
	Total  int64
	Rows   []*Row
	Fields []*Field // for Rows is empty
	Error  error
}

func (r *Result) NumPages() int {
	page_size := lo.Ternary(r.PageSize != 0, r.PageSize, r.default_page_size)
	return int(1 + (r.Total-1)/int64(page_size))
}

type pager struct {
	Text     string
	Href     string
	Disabled bool
	Active   bool
}

func (r *Result) PageItems() []pager {
	n := r.NumPages()

	low, up := r.Page-3, r.Page+4
	if low < 0 {
		up = up - low
	}
	if up > n {
		low = low - up + n
	}

	low = max(low, 0)
	up = min(up, n)

	notLink := "#" // TODO: javascript:void(0)? go template escape as "ZgotmplZ"

	res := make([]pager, 0, 7)
	// « = &laquo
	if low > 0 {
		res = append(res, pager{Text: "«", Href: r.urlForPage(0)})
	} else {
		res = append(res, pager{Text: "«", Href: notLink, Disabled: true})
	}

	if r.Page > 0 {
		res = append(res, pager{Text: "<", Href: r.urlForPage(r.Page - 1)})
	} else {
		res = append(res, pager{Text: "<", Href: notLink, Disabled: true})
	}

	for i := low; i < up; i++ {
		if i == r.Page {
			res = append(res, pager{Text: cast.ToString(i + 1), Href: notLink, Active: true})
		} else {
			res = append(res, pager{Text: cast.ToString(i + 1), Href: r.urlForPage(i)})
		}
	}

	if r.Page+1 < n {
		res = append(res, pager{Text: ">", Href: r.urlForPage(r.Page + 1)})
	} else {
		res = append(res, pager{Text: ">", Href: notLink, Disabled: true})
	}

	if up < n {
		res = append(res, pager{Text: "»", Href: r.urlForPage(n - 1)})
	} else {
		res = append(res, pager{Text: "»", Href: notLink, Disabled: true})
	}
	return res
}

var pagerTemplate *template.Template

func init() {
	if _, err := os.Stat("templates/pager.tmpl"); err == nil {
		pagerTemplate = parseTemplate("pager", nil, "templates/pager.tmpl")
	}
}

func (r *Result) PagerHtml() template.HTML {
	w := bytes.Buffer{}
	if err := pagerTemplate.ExecuteTemplate(&w, "npager", r); err != nil {
		panic(err)
	}
	return template.HTML(w.String())
}
