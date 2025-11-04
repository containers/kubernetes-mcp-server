package promptsets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type PromptSetsSuite struct {
	suite.Suite
}

func (s *PromptSetsSuite) SetupTest() {
	// Clear the registry before each test
	Clear()
}

func (s *PromptSetsSuite) TestRegister() {
	// Given
	testPS := &testPromptSet{name: "test"}

	// When
	Register(testPS)

	// Then
	assert.Equal(s.T(), 1, len(PromptSets()))
	assert.Equal(s.T(), testPS, PromptSets()[0])
}

func (s *PromptSetsSuite) TestPromptSetFromString() {
	s.Run("Returns nil if promptset not found", func() {
		// When
		ps := PromptSetFromString("nonexistent")

		// Then
		assert.Nil(s.T(), ps)
	})

	s.Run("Returns the correct promptset if found", func() {
		// Given
		testPS := &testPromptSet{name: "test"}
		Register(testPS)

		// When
		ps := PromptSetFromString("test")

		// Then
		assert.Equal(s.T(), testPS, ps)
		assert.Equal(s.T(), "test", ps.GetName())
	})

	s.Run("Returns the correct promptset if found after trimming spaces", func() {
		// Given
		testPS := &testPromptSet{name: "test"}
		Register(testPS)

		// When
		ps := PromptSetFromString("  test  ")

		// Then
		assert.Equal(s.T(), testPS, ps)
	})
}

func (s *PromptSetsSuite) TestAllPromptSets() {
	// Given
	testPS1 := &testPromptSet{name: "test1"}
	testPS2 := &testPromptSet{name: "test2"}
	Register(testPS1)
	Register(testPS2)

	// When
	all := AllPromptSets()

	// Then
	assert.Equal(s.T(), 2, len(all))
	assert.Contains(s.T(), all, testPS1)
	assert.Contains(s.T(), all, testPS2)
}

func (s *PromptSetsSuite) TestGetPromptSetNames() {
	s.Run("Returns empty slice when no promptsets registered", func() {
		// When
		names := GetPromptSetNames()

		// Then
		assert.Empty(s.T(), names)
	})

	s.Run("Returns sorted names of all registered promptsets", func() {
		// Given
		Register(&testPromptSet{name: "zebra"})
		Register(&testPromptSet{name: "alpha"})
		Register(&testPromptSet{name: "beta"})

		// When
		names := GetPromptSetNames()

		// Then
		assert.Equal(s.T(), []string{"alpha", "beta", "zebra"}, names)
	})
}

func TestPromptSets(t *testing.T) {
	suite.Run(t, new(PromptSetsSuite))
}

// Test helper
type testPromptSet struct {
	name string
}

func (t *testPromptSet) GetName() string {
	return t.name
}

func (t *testPromptSet) GetDescription() string {
	return "Test promptset"
}

func (t *testPromptSet) GetPrompts(o internalk8s.Openshift) []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Name:        "test_prompt",
			Description: "Test prompt",
			Arguments:   []api.PromptArgument{},
			GetMessages: func(arguments map[string]string) []api.PromptMessage {
				return []api.PromptMessage{
					{Role: "user", Content: "test"},
				}
			},
		},
	}
}
