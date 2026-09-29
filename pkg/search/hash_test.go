package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

const sampleHex = "0123456789abcdef0123456789abcdef01234567"

// sampleBase32 is sampleHex in the base32 form magnets sometimes use.
const sampleBase32 = "AERUKZ4JVPG66AJDIVTYTK6N54ASGRLH"

func TestNormalizeHash(t *testing.T) {
	assert.Equal(t, sampleHex, search.NormalizeHash(sampleHex))
	assert.Equal(t, sampleHex, search.NormalizeHash(" 0123456789ABCDEF0123456789ABCDEF01234567 "))
	assert.Equal(t, sampleHex, search.NormalizeHash(sampleBase32))
	assert.Equal(t, sampleHex, search.NormalizeHash("aeruKZ4JVPG66AJDIVTYTK6N54ASGRLH"))
	assert.Empty(t, search.NormalizeHash(""))
	assert.Empty(t, search.NormalizeHash("zz23456789abcdef0123456789abcdef01234567"))
	assert.Empty(t, search.NormalizeHash("0123456789abcdef"))
	assert.Empty(t, search.NormalizeHash("11111111111111111111111111111111"), "1 is not a base32 digit")
}

func TestHashFromMagnet(t *testing.T) {
	assert.Equal(t, sampleHex, search.HashFromMagnet("magnet:?xt=urn:btih:"+sampleHex+"&dn=Sample"))
	assert.Equal(t, sampleHex, search.HashFromMagnet("magnet:?dn=x&xt=urn:BTIH:"+sampleBase32))
	assert.Equal(t, sampleHex, search.HashFromMagnet("magnet:?xt=urn:btmh:1220abcd&xt=urn:btih:"+sampleHex))
	assert.Empty(t, search.HashFromMagnet("https://example.invalid/?xt=urn:btih:"+sampleHex))
	assert.Empty(t, search.HashFromMagnet("magnet:?xt=urn:btih:nothex"))
	assert.Empty(t, search.HashFromMagnet("magnet:?dn=no-hash"))
}

func TestMagnet(t *testing.T) {
	assert.Equal(t, "magnet:?xt=urn:btih:"+sampleHex+"&dn=Sample+Film+%282014%29", search.Magnet(sampleHex, "Sample Film (2014)"))
	assert.Equal(t, "magnet:?xt=urn:btih:"+sampleHex, search.Magnet(sampleHex, ""))
	assert.Equal(t, sampleHex, search.HashFromMagnet(search.Magnet(sampleHex, "a&b=c")))
}
