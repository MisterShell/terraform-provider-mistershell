// Package apimanaged implements the explicit, reviewed configuration contracts
// in contracts.json. Paths and field ownership are fixed by the provider, never
// supplied as arbitrary HTTP requests in Terraform configuration.
package apimanaged

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed contracts.json
var contractsJSON []byte

type Field struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Description string          `json:"description"`
	Create      bool            `json:"create"`
	Update      bool            `json:"update"`
	Required    bool            `json:"required"`
	Computed    bool            `json:"computed"`
	Immutable   bool            `json:"immutable"`
	Sensitive   bool            `json:"sensitive"`
	SecretKeys  []string        `json:"secret_keys,omitempty"`
	Default     json.RawMessage `json:"default"`
	Enum        []string        `json:"enum"`
	MinLength   int             `json:"min_length"`
	MaxLength   int             `json:"max_length"`
}

func (f Field) Writable() bool { return f.Create || f.Update }

type Definition struct {
	Name         string  `json:"name"`
	ListOnly     bool    `json:"list_only"`
	Path         string  `json:"path"`
	ItemPath     string  `json:"item_path"`
	ListPath     string  `json:"list_path"`
	Method       string  `json:"method"`
	UpdateMethod string  `json:"update_method"`
	Description  string  `json:"description"`
	Fields       []Field `json:"fields"`
}

func Definitions() []Definition {
	var defs []Definition
	if err := json.Unmarshal(contractsJSON, &defs); err != nil {
		panic(fmt.Sprintf("invalid embedded contracts: %v", err))
	}
	return defs
}
