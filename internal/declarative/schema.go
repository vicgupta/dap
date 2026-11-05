package declarative

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	jschema "github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"

	"github.com/vicgupta/dap/schemas"
)

// SchemaErrors checks plugin.yaml against schemas/declarative.schema.json.
// Semantic rules (durations, setting references, llm) are checked by Compile.
func SchemaErrors(yamlBytes []byte) []string {
	var raw any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return []string{"manifest is not valid YAML: " + err.Error()}
	}
	doc, err := yamlToJSON(raw)
	if err != nil {
		return []string{err.Error()}
	}
	compiler := jschema.NewCompiler()
	if err := compiler.AddResource("https://dap.dev/schemas/declarative.schema.json", strings.NewReader(schemas.Declarative)); err != nil {
		return []string{"schema: " + err.Error()}
	}
	schema, err := compiler.Compile("https://dap.dev/schemas/declarative.schema.json")
	if err != nil {
		return []string{"schema: " + err.Error()}
	}
	if err := schema.Validate(doc); err != nil {
		return flattenSchema(err)
	}
	return nil
}

func yamlToJSON(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("manifest could not be read as JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func flattenSchema(err error) []string {
	ve, ok := err.(*jschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	walkSchema(ve, &out)
	if len(out) == 0 {
		out = append(out, ve.Error())
	}
	return out
}

func walkSchema(ve *jschema.ValidationError, out *[]string) {
	if len(ve.Causes) == 0 {
		where := ve.InstanceLocation
		if where == "" {
			where = "/"
		}
		msg := ve.Message
		if msg == "" {
			msg = ve.Error()
		}
		*out = append(*out, where+": "+msg)
		return
	}
	for _, c := range ve.Causes {
		walkSchema(c, out)
	}
}
