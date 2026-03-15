//go:build tools

// Package tools tracks go generate tool dependencies so that go mod tidy
// keeps them in go.mod even though they are not imported in regular code.
package tools

import (
	// Used by go:generate in main.go to build provider documentation.
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
