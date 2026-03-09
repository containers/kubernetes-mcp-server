package core_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
)

var validLabelSelectors = []struct{ selector string }{
	{"app"},
	{"!app"},

	{"app=myapp"},
	{"app=myapp,env=prod"},
	{"app.kubernetes.io/name=myapp"},
	{"node-role.kubernetes.io/worker="},

	{"app in (myapp,yourapp)"},
	{"environment in (production),tier in (frontend)"},
	{"environment in (production, qa)"},
	{"environment,environment notin (frontend)"},
}

func Test_LabelSelectorRegex_is_valid(t *testing.T) {
	for _, tc := range validLabelSelectors {
		t.Run(fmt.Sprint("Selector should be valid: ", tc.selector), func(t *testing.T) {
			if match, _ := regexp.MatchString(core.REGEX_LABELSELECTOR_VALID_CHARS, tc.selector); !match {
				t.Errorf("Pattern %s did not match valid selector: %s", core.REGEX_LABELSELECTOR_VALID_CHARS, tc.selector)
			}
		})
	}
}

var validFieldSelectors = []struct{ selector string }{
	{"metadata.name=myresource"},
	{"status.phase=Running"},
	{"metadata.namespace!=default"},
	{"metadata.name=my-service"},
}

func Test_FieldSelectorRegex_is_valid(t *testing.T) {
	for _, tc := range validFieldSelectors {
		t.Run(fmt.Sprint("Selector should be valid: ", tc.selector), func(t *testing.T) {
			if match, _ := regexp.MatchString(core.REGEX_FIELDSELECTOR, tc.selector); !match {
				t.Errorf("Pattern %s did not match valid selector: %s", core.REGEX_FIELDSELECTOR, tc.selector)
			}
		})
	}
}
