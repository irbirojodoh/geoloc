package storage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey("avatars", "user-123", "photo.JPG", "image/jpeg")
	require.NoError(t, err)
	assert.True(t, IsMediaKey(key))
	assert.Contains(t, key, "avatars/user-123/")
	assert.True(t, len(key) > len("avatars/user-123/"))
}

func TestGenerateKeyInvalidFolder(t *testing.T) {
	_, err := GenerateKey("private", "user-123", "photo.jpg", "image/jpeg")
	require.Error(t, err)
}

func TestGenerateKeyInvalidContentType(t *testing.T) {
	_, err := GenerateKey("posts", "user-123", "video.mp4", "video/mp4")
	require.Error(t, err)
}

func TestValidateKeyRejectsTraversal(t *testing.T) {
	err := ValidateKey("avatars/user-123/../secret.jpg")
	require.Error(t, err)
}

func TestValidateKeyValid(t *testing.T) {
	err := ValidateKey("posts/user-123/550e8400-e29b-41d4-a716-446655440000.jpg")
	require.NoError(t, err)
}

func TestKeyOwnerUserID(t *testing.T) {
	owner, err := KeyOwnerUserID("covers/abc-456/file.webp")
	require.NoError(t, err)
	assert.Equal(t, "abc-456", owner)
}

func TestExtensionFromFilenameOrContentType(t *testing.T) {
	assert.Equal(t, ".png", ExtensionFromFilenameOrContentType("pic.PNG", "image/jpeg"))
	assert.Equal(t, ".webp", ExtensionFromFilenameOrContentType("", "image/webp"))
}

func TestKeyFromStoredValue(t *testing.T) {
	domain := "https://cdn.example.com"
	assert.Equal(t, "avatars/u1/f.jpg", KeyFromStoredValue("avatars/u1/f.jpg", domain))
	assert.Equal(t, "avatars/u1/f.jpg", KeyFromStoredValue("https://cdn.example.com/avatars/u1/f.jpg", domain))
}

func TestResolveMediaURLWithProxy(t *testing.T) {
	t.Setenv("BASE_URL", "https://api.geoloc.app")
	store := NewMemoryMediaStore("")
	key := "avatars/user-1/test.jpg"
	require.NoError(t, store.PutObject(t.Context(), key, strings.NewReader("data"), 4, "image/jpeg"))

	url := ResolveMediaURL(store, key)
	assert.Equal(t, "https://api.geoloc.app/api/v1/media/file?key=avatars/user-1/test.jpg", url)

	stored := StoredMediaValue(store, key)
	assert.Equal(t, "avatars/user-1/test.jpg", stored)
}

func TestResolveMediaURLEmpty(t *testing.T) {
	assert.Equal(t, "", ResolveMediaURL(nil, ""))
}
