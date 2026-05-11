package defaults

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProductNameReturnsDefault(t *testing.T) {
	assert.NotEmpty(t, ProductName())
}

func TestToolsetDescriptionReturnsDefault(t *testing.T) {
	assert.NotEmpty(t, ToolsetDescription())
}
