package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("rahasia-banget")
	require.NoError(t, err)

	assert.NotEqual(t, "rahasia-banget", h)
	assert.True(t, Verify(h, "rahasia-banget"))
	assert.False(t, Verify(h, "salah"))
}

func TestHash_SaltedSoHashesDiffer(t *testing.T) {
	a, _ := Hash("password-sama")
	b, _ := Hash("password-sama")
	assert.NotEqual(t, a, b)
}

func TestHash_RejectsShortAndTooLong(t *testing.T) {
	_, err := Hash("pendek")
	assert.ErrorIs(t, err, ErrTooShort)

	_, err = Hash(strings.Repeat("a", 73))
	assert.Error(t, err)
}

func TestVerify_InvalidHashIsFalse(t *testing.T) {
	assert.False(t, Verify("bukan-hash", "apa-saja"))
	assert.False(t, Verify("", ""))
}
