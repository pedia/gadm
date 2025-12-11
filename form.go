package gadm

import (
	"encoding/json"
	"html/template"

	"github.com/spf13/cast"
	"gorm.io/gorm/schema"
)

// <a data-csrf="" data-pk="5ad19739-80a1-4b0e-9b6d-ab7e264bd4eb"
// data-role="x-editable" data-type="text" data-url="./ajax/update/"
// data-value="EUR" href="#" id="currency" name="currency">EUR</a>
func InlineEdit(gt *groupTempl, token string, model *Model, field *Field, row *Row) template.HTML {
	dv := field.Display()
	args := map[template.HTMLAttr]any{
		"data-value": dv,
		"data-role":  "x-editable", // x-editable-boolean, x-editable-combodate data-template
		"data-url":   "ajax/update",
		"data-pk":    row.GetPkValue(),
		"data-csrf":  token,
		"data-type":  "text",
		"id":         field.DBName,
		"name":       field.DBName,
		"href":       "#",
	}

	if field.Choices != nil {
		args["data-type"] = "select2"
		args["data-source"] = jsonify(field.Choices)
	}
	if field.TextAreaRow > 0 {
		args["data-type"] = "textarea"
		args["data-row"] = cast.ToString(field.TextAreaRow)
	}

	switch field.DataType {
	case schema.Time:
		args["data-type"] = "combodate"
		args["data-template"] = "YYYY-MM-DD" // TODO
		args["data-role"] = "x-editable-combodate"
	case schema.Int, schema.Uint, schema.Float:
		args["data-type"] = "number"
	case schema.Bool:
		args["data-type"] = "select2"
		args["data-role"] = "x-editable-boolean"
		args["data-source"] = `[{"text": "False", "value": "false"},{"text": "True", "value": "true"}]`
	}

	return gt.Execute("inline_field", map[string]any{
		"args":          args,
		"display_value": dv,
		"field":         field,
	})
}

func jsonify(a any) string {
	if bs, err := json.Marshal(a); err == nil {
		return string(bs)
	}
	return ""
}

type modelForm struct {
	Fields    []*Field
	Row       *Row
	CSRFToken string
}

func NewForm(fs []*Field, row *Row, csrfToken string) *modelForm {
	return &modelForm{Fields: fs, Row: row, CSRFToken: csrfToken}
}
