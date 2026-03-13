package output

import (
	"bytes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	yml "sigs.k8s.io/yaml"
)

var Yaml = &yaml{}

var Table = &table{}

// PrintResult holds both the text representation and optional structured data
// extracted from a Kubernetes object.
type PrintResult struct {
	// Text is the human-readable formatted output (YAML or Table).
	Text string
	// Structured is an optional JSON-serializable value extracted from the object.
	// For Table output, this is []map[string]any with column headers as keys.
	// For YAML output, this is the cleaned-up object items as []map[string]any (lists)
	// or a single map[string]any (individual objects).
	Structured any
}

type Output interface {
	// GetName returns the name of the output format, will be used by the CLI to identify the output format.
	GetName() string
	// AsTable true if the kubernetes request should be made with the `application/json;as=Table;v=0.1` header.
	AsTable() bool
	// PrintObj prints the given object as a string.
	PrintObj(obj runtime.Unstructured) (string, error)
	// PrintObjStructured prints the given object and also extracts structured data for MCP Apps UI rendering.
	PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error)
}

var Outputs = []Output{
	Yaml,
	Table,
}

var Names []string

func FromString(name string) Output {
	for _, output := range Outputs {
		if output.GetName() == name {
			return output
		}
	}
	return nil
}

type yaml struct{}

func (p *yaml) GetName() string {
	return "yaml"
}
func (p *yaml) AsTable() bool {
	return false
}
func (p *yaml) PrintObj(obj runtime.Unstructured) (string, error) {
	return MarshalYaml(obj)
}
func (p *yaml) PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error) {
	text, err := p.PrintObj(obj)
	if err != nil {
		return nil, err
	}
	switch t := obj.(type) {
	case *unstructured.UnstructuredList:
		items := make([]map[string]any, 0, len(t.Items))
		for _, item := range t.Items {
			items = append(items, item.Object)
		}
		return &PrintResult{Text: text, Structured: items}, nil
	case *unstructured.Unstructured:
		return &PrintResult{Text: text, Structured: t.Object}, nil
	}
	return &PrintResult{Text: text}, nil
}

type table struct{}

func (p *table) GetName() string {
	return "table"
}
func (p *table) AsTable() bool {
	return true
}
func (p *table) PrintObj(obj runtime.Unstructured) (string, error) {
	text, _, err := p.printTable(obj)
	return text, err
}

func (p *table) PrintObjStructured(obj runtime.Unstructured) (*PrintResult, error) {
	text, t, err := p.printTable(obj)
	if err != nil {
		return nil, err
	}
	// Guard against typed nil leaking into the any interface — a nil []map[string]any
	// assigned to Structured (type any) would create a non-nil interface, causing
	// downstream nil checks (e.g. in NewStructuredResult) to incorrectly pass.
	if structured := tableToStructured(t); structured != nil {
		return &PrintResult{Text: text, Structured: structured}, nil
	}
	return &PrintResult{Text: text}, nil
}

// printTable formats the object as a table and returns the text, the parsed Table (if available), and any error.
func (p *table) printTable(obj runtime.Unstructured) (string, *metav1.Table, error) {
	var objectToPrint runtime.Object = obj
	var parsedTable *metav1.Table
	withNamespace := false
	if obj.GetObjectKind().GroupVersionKind() == metav1.SchemeGroupVersion.WithKind("Table") {
		t := &metav1.Table{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), t); err == nil {
			objectToPrint = t
			parsedTable = t
			// Process the Raw object to retrieve the complete metadata (see kubectl/pkg/printers/table_printer.go)
			for i := range t.Rows {
				row := &t.Rows[i]
				if row.Object.Raw == nil || row.Object.Object != nil {
					continue
				}
				row.Object.Object, err = runtime.Decode(unstructured.UnstructuredJSONScheme, row.Object.Raw)
				// Print namespace if at least one row has it (object is namespaced)
				if err == nil && !withNamespace {
					switch rowObject := row.Object.Object.(type) {
					case *unstructured.Unstructured:
						withNamespace = rowObject.GetNamespace() != ""
					}
				}
			}
		}
	}
	buf := new(bytes.Buffer)
	// TablePrinter is mutable and not thread-safe, must create a new instance each time.
	printer := printers.NewTablePrinter(printers.PrintOptions{
		WithNamespace: withNamespace,
		WithKind:      true,
		Wide:          true,
		ShowLabels:    true,
	})
	err := printer.PrintObj(objectToPrint, buf)
	return buf.String(), parsedTable, err
}

// tableToStructured converts a Kubernetes Table response to []map[string]any
// using column definitions as keys.
func tableToStructured(t *metav1.Table) []map[string]any {
	if t == nil || len(t.Rows) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(t.Rows))
	for _, row := range t.Rows {
		item := make(map[string]any, len(t.ColumnDefinitions))
		for ci, col := range t.ColumnDefinitions {
			if ci < len(row.Cells) {
				item[col.Name] = row.Cells[ci]
			}
		}
		// Add namespace from the embedded object metadata if available
		if row.Object.Object != nil {
			if u, ok := row.Object.Object.(*unstructured.Unstructured); ok {
				if ns := u.GetNamespace(); ns != "" {
					item["Namespace"] = ns
				}
			}
		}
		result = append(result, item)
	}
	return result
}

func MarshalYaml(v any) (string, error) {
	switch t := v.(type) {
	//case unstructured.UnstructuredList:
	//	for i := range t.Items {
	//		t.Items[i].SetManagedFields(nil)
	//	}
	//	v = t.Items
	case *unstructured.UnstructuredList:
		for i := range t.Items {
			t.Items[i].SetManagedFields(nil)
		}
		v = t.Items
	//case unstructured.Unstructured:
	//	t.SetManagedFields(nil)
	case *unstructured.Unstructured:
		t.SetManagedFields(nil)
	}
	ret, err := yml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(ret), nil
}

func init() {
	Names = make([]string, 0)
	for _, output := range Outputs {
		Names = append(Names, output.GetName())
	}
}
