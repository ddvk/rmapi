package shell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTags(t *testing.T) {
	assert.Nil(t, parseTags(""))
	assert.Nil(t, parseTags(" , ,"))
	assert.Equal(t, []string{"book"}, parseTags("book"))
	assert.Equal(t, []string{"book", "fiction"}, parseTags("book,fiction"))
	assert.Equal(t, []string{"book", "to read"}, parseTags(" book , to read , "))
}
