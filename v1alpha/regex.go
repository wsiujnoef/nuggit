package v1alpha

import (
	"context"
	"fmt"
	"regexp"

	"github.com/wenooij/nuggit/runtime"
)

// Regex defines a Go-style regular expression.
//
// Pattern should be a string input the regular expression.
//
// The pattern can incorporate steps and variables using
// step inputs.
//
// Syntax: https://golang.org/s/re2syntax.
type Regex struct {
	Pattern string `json:"pattern,omitempty"`
}

func (x *Regex) Bind(e runtime.Edge) error {
	switch e.SrcField {
	case "pattern":
		x.Pattern = e.Result.(string)
	case "":
		*x = *e.Result.(*Regex)
	default:
		return fmt.Errorf("not found: %q", e.SrcField)
	}
	return nil
}

func (x *Regex) Run(ctx context.Context) (any, error) {
	if x.Pattern == "" {
		return nil, fmt.Errorf("missing Pattern")
	}
	r, err := regexp.Compile(x.Pattern)
	if err != nil {
		return nil, err
	}
	return r, nil
}
