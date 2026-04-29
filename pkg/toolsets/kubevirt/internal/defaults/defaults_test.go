package defaults

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProductNameReturnsDefault(t *testing.T) {
	// When no override is set, ProductName returns the default
	assert.Equal(t, DefaultProductName, ProductName())
}

func TestToolsetDescriptionReturnsDefault(t *testing.T) {
	// When no override is set, ToolsetDescription returns the default
	assert.Equal(t, DefaultToolsetDescription, ToolsetDescription())
}
