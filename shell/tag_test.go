package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeTags(t *testing.T) {
	assert.Equal(t, []string{"case"}, mergeTags(nil, []string{"case"}))
	assert.Equal(t, []string{"case", "emba"}, mergeTags([]string{"case"}, []string{"emba"}))
	assert.Equal(t, []string{"case", "emba"}, mergeTags([]string{"case", "emba"}, []string{"case"}))
	assert.Equal(t, []string{"a", "b"}, mergeTags([]string{"a"}, []string{"b", "a", "b"}))
}

func TestRemoveTags(t *testing.T) {
	assert.Equal(t, []string{}, removeTags(nil, []string{"case"}))
	assert.Equal(t, []string{"emba"}, removeTags([]string{"case", "emba"}, []string{"case"}))
	assert.Equal(t, []string{"case", "emba"}, removeTags([]string{"case", "emba"}, []string{"other"}))
}
