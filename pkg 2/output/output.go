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

type Output interface {
	// GetName returns the name of the output format, will be used by the CLI to identify the output format.
	GetName() string
	// AsTable true if the kubernetes request should be made with the `application/json;as=Table;v=0.1` header.
	AsTable() bool
	// PrintObj prints the given object as a string.
	PrintObj(obj runtime.Unstructured) (string, error)
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

type table struct{}

func (p *table) GetName() string {
	return "table"
}
func (p *table) AsTable() bool {
	return true
}
func (p *table) PrintObj(obj runtime.Unstructured) (string, error) {
	var objectToPrint runtime.Object = obj
	withNamespace := false
	if obj.GetObjectKind().GroupVersionKind() == metav1.SchemeGroupVersion.WithKind("Table") {
		t := &metav1.Table{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), t); err == nil {
			objectToPrint = t
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
	return buf.String(), err
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
