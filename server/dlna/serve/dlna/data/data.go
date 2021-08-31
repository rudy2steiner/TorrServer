package data

import (
	"text/template"

	"github.com/pkg/errors"
)

// GetTemplate returns the rootDesc XML template
func GetTemplate() (tpl *template.Template, err error) {

	var templateString = string(rootDescTpl)

	tpl, err = template.New("rootDesc").Parse(templateString)
	if err != nil {
		return nil, errors.Wrap(err, "get template parse")
	}

	return
}
