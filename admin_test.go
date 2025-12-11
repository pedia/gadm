package gadm

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/leonelquinteros/gotext.v1"
)

func TestPager(t *testing.T) {
	is := assert.New(t)
	r := Result{Query: &Query{Page: 1, PageSize: 10, default_page_size: 10}, Total: 100}
	is.Equal(10, r.NumPages())
	ps := r.PageItems()
	is.Len(ps, 11)
	is.NotEmpty(r.PagerHtml())

	r = Result{Query: &Query{Page: 5, PageSize: 10}, Total: 100}
	r = Result{Query: &Query{Page: 9, PageSize: 10}, Total: 100}
	r = Result{Query: &Query{Page: 0, PageSize: 10}, Total: 4}
	r = Result{Query: &Query{Page: 0, PageSize: 10}, Total: 11}
	r = Result{Query: &Query{Page: 1, PageSize: 10}, Total: 11}
	r = Result{Query: &Query{Page: 0, PageSize: 1}, Total: 1}
}

func TestText(t *testing.T) {
	is := assert.New(t)

	gotext.Configure("translations", "zh_Hant_TW", "admin")
	is.Equal("首頁", gotext.Get("Home"))
	is.Equal(`檔案 "foo" 已經存在。`, gotext.Get(`File "%s" already exists.`, "foo"))
}

func TestGen(t *testing.T) {
	is := assert.New(t)

	g := NewGenerator("sqlite:examples/sqla/sample.db")

	err := g.Run(nil, io.Discard)
	is.Nil(err)
}
