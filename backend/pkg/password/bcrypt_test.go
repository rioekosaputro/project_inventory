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

func TestGenerate_LengthAlphabetUniquenessAndHashable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		p, err := Generate(16)
		require.NoError(t, err)
		assert.Len(t, p, 16)
		for _, r := range p {
			assert.Contains(t, tempAlphabet, string(r), "karakter di luar alfabet: %q", r)
		}
		assert.False(t, seen[p], "password acak tidak boleh berulang")
		seen[p] = true
	}

	p, _ := Generate(16)
	h, err := Hash(p)
	require.NoError(t, err, "hasil Generate harus lolos aturan Hash")
	assert.True(t, Verify(h, p))
}

func TestGenerate_RejectsTooShort(t *testing.T) {
	_, err := Generate(MinLength - 1)
	assert.ErrorIs(t, err, ErrTooShort)
}

func TestGenerate_AvoidsAmbiguousCharacters(t *testing.T) {
	for _, c := range "0OoIl1" {
		assert.NotContains(t, tempAlphabet, string(c))
	}
}

func TestVerify_InvalidHashIsFalse(t *testing.T) {
	assert.False(t, Verify("bukan-hash", "apa-saja"))
	assert.False(t, Verify("", ""))
}
