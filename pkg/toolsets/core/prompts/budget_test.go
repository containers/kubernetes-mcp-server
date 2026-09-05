package prompts

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

type BudgetSuite struct {
	suite.Suite
}

func (s *BudgetSuite) TestCountEventLines() {
	s.Run("nil results returns zero", func() {
		s.Equal(0, countEventLines(nil))
	})

	s.Run("errors and nil outputs are ignored", func() {
		results := []tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Err: errAny},
			{Name: eventTaskPrefix + "ns2", Output: nil},
		}
		s.Equal(0, countEventLines(results))
	})

	s.Run("counts object headers and entries", func() {
		results := []tasks.TaskResult{
			{
				Name: eventTaskPrefix + "ns1",
				Output: &eventReport{
					objects: []objectEvents{
						{key: "a", entries: []eventEntry{{}, {}}},
						{key: "b", entries: []eventEntry{{}}},
					},
				},
			},
		}
		// 2 objects: (1+2) + (1+1) = 5
		s.Equal(5, countEventLines(results))
	})

	s.Run("non-event outputs are ignored", func() {
		results := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{issues: []podIssue{{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}}}}},
		}
		s.Equal(0, countEventLines(results))
	})
}

func (s *BudgetSuite) TestCountNamespaceHealthLines() {
	s.Run("nil results returns zero", func() {
		s.Equal(0, countNamespaceHealthLines(nil))
	})

	s.Run("counts distinct namespaces across report types", func() {
		results := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{issues: []podIssue{
				{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}},
				{namespace: "ns2", issues: map[string][]string{"x": {"p2"}}},
			}}},
			{Name: taskNameDeployments, Output: &workloadHealthReport{workloadType: taskNameDeployments, items: []workloadIssue{
				{namespace: "ns2", unhealthy: map[string]int32{"d1": 1}},
				{namespace: "ns3", unhealthy: map[string]int32{"d2": 1}},
			}}},
			{Name: taskNamePVCs, Output: &pvcHealthReport{items: []pvcIssue{
				{namespace: "ns1", pending: []string{"pvc1"}},
			}}},
		}
		s.Equal(3, countNamespaceHealthLines(results))
	})

	s.Run("empty reports contribute no namespaces", func() {
		results := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{totalPods: 5}},
			{Name: taskNameDeployments, Output: &workloadHealthReport{workloadType: taskNameDeployments}},
		}
		s.Equal(0, countNamespaceHealthLines(results))
	})

	s.Run("errors and nil outputs are ignored", func() {
		results := []tasks.TaskResult{
			{Name: taskNamePods, Err: errAny},
			{Name: taskNamePods, Output: nil},
		}
		s.Equal(0, countNamespaceHealthLines(results))
	})
}

func (s *BudgetSuite) TestAllocateReportBudgets() {
	s.Run("no namespace entries gives full budget to events", func() {
		eventResults := []tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Output: &eventReport{objects: []objectEvents{
				{key: "a", entries: make([]eventEntry, 250)},
			}}},
		}
		nsBudget, eventBudget := allocateReportBudgets(nil, eventResults)
		s.Equal(0, nsBudget)
		s.Equal(reportLineLimit, eventBudget)
	})

	s.Run("single namespace reserves only its own line count", func() {
		nsResults := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{issues: []podIssue{{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}}}}},
		}
		// More than enough events to consume the remainder.
		eventResults := []tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Output: &eventReport{objects: []objectEvents{
				{key: "a", entries: make([]eventEntry, reportLineLimit)},
			}}},
		}
		nsBudget, eventBudget := allocateReportBudgets(nsResults, eventResults)
		s.Equal(1, nsBudget)
		s.Equal(reportLineLimit-1, eventBudget)
	})

	s.Run("small event count leaves remaining budget for namespaces", func() {
		nsResults := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{issues: []podIssue{{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}}}}},
		}
		eventResults := []tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Output: &eventReport{objects: []objectEvents{
				{key: "a", entries: make([]eventEntry, 10)},
			}}},
		}
		nsBudget, eventBudget := allocateReportBudgets(nsResults, eventResults)
		s.Equal(11, eventBudget)
		s.Equal(reportLineLimit-11, nsBudget)
	})

	s.Run("namespace reservation is capped at maxNamespaceReportLines", func() {
		nsResults := []tasks.TaskResult{
			{
				Name: taskNamePods,
				Output: &podHealthReport{issues: []podIssue{
					{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}},
					{namespace: "ns2", issues: map[string][]string{"x": {"p2"}}},
					{namespace: "ns3", issues: map[string][]string{"x": {"p3"}}},
					{namespace: "ns4", issues: map[string][]string{"x": {"p4"}}},
					{namespace: "ns5", issues: map[string][]string{"x": {"p5"}}},
					{namespace: "ns6", issues: map[string][]string{"x": {"p6"}}},
					{namespace: "ns7", issues: map[string][]string{"x": {"p7"}}},
					{namespace: "ns8", issues: map[string][]string{"x": {"p8"}}},
					{namespace: "ns9", issues: map[string][]string{"x": {"p9"}}},
					{namespace: "ns10", issues: map[string][]string{"x": {"p10"}}},
					{namespace: "ns11", issues: map[string][]string{"x": {"p11"}}},
					{namespace: "ns12", issues: map[string][]string{"x": {"p12"}}},
					{namespace: "ns13", issues: map[string][]string{"x": {"p13"}}},
					{namespace: "ns14", issues: map[string][]string{"x": {"p14"}}},
					{namespace: "ns15", issues: map[string][]string{"x": {"p15"}}},
					{namespace: "ns16", issues: map[string][]string{"x": {"p16"}}},
					{namespace: "ns17", issues: map[string][]string{"x": {"p17"}}},
					{namespace: "ns18", issues: map[string][]string{"x": {"p18"}}},
					{namespace: "ns19", issues: map[string][]string{"x": {"p19"}}},
					{namespace: "ns20", issues: map[string][]string{"x": {"p20"}}},
					{namespace: "ns21", issues: map[string][]string{"x": {"p21"}}},
					{namespace: "ns22", issues: map[string][]string{"x": {"p22"}}},
					{namespace: "ns23", issues: map[string][]string{"x": {"p23"}}},
					{namespace: "ns24", issues: map[string][]string{"x": {"p24"}}},
					{namespace: "ns25", issues: map[string][]string{"x": {"p25"}}},
				}},
			},
		}
		eventResults := []tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Output: &eventReport{objects: []objectEvents{
				{key: "a", entries: make([]eventEntry, reportLineLimit)},
			}}},
		}
		nsBudget, eventBudget := allocateReportBudgets(nsResults, eventResults)
		s.Equal(maxNamespaceReportLines, nsBudget)
		s.Equal(reportLineLimit-maxNamespaceReportLines, eventBudget)
	})

	s.Run("zero events leaves full budget for namespaces", func() {
		nsResults := []tasks.TaskResult{
			{Name: taskNamePods, Output: &podHealthReport{issues: []podIssue{{namespace: "ns1", issues: map[string][]string{"x": {"p1"}}}}}},
		}
		nsBudget, eventBudget := allocateReportBudgets(nsResults, nil)
		s.Equal(0, eventBudget)
		s.Equal(reportLineLimit, nsBudget)
	})
}

var errAny = errors.New("any error")

func TestBudget(t *testing.T) {
	suite.Run(t, new(BudgetSuite))
}
