package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/storage"
)

func TestMediaSignURLRejectsTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("https://cdn.test")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.GET("/media/sign", auth.AuthRequired(), h.SignURL)

	token := testAccessToken(t, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/media/sign?key=avatars/u1/../secret.jpg", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMediaDeleteObjectOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.DELETE("/media/object", auth.AuthRequired(), h.DeleteObject)

	key := "avatars/owner-1/550e8400-e29b-41d4-a716-446655440000.jpg"
	require.NoError(t, store.PutObject(t.Context(), key, strings.NewReader("x"), 1, "image/jpeg"))

	token := testAccessToken(t, "other-user")

	req := httptest.NewRequest(http.MethodDelete, "/media/object?key="+key, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestMediaUploadURLFolderAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.POST("/media/upload-url", auth.AuthRequired(), h.UploadURL)

	token := testAccessToken(t, "user-1")

	body := `{"folder":"tmp","content_type":"image/jpeg","filename":"a.jpg"}`
	req := httptest.NewRequest(http.MethodPost, "/media/upload-url", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMediaUploadURLSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.POST("/media/upload-url", auth.AuthRequired(), h.UploadURL)

	token := testAccessToken(t, "user-abc")

	body := `{"folder":"posts","content_type":"image/png","filename":"pic.png"}`
	req := httptest.NewRequest(http.MethodPost, "/media/upload-url", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["key"], "posts/user-abc/")
	assert.NotEmpty(t, resp["upload_url"])
}

func TestMediaServeMediaSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.GET("/media/file", auth.AuthRequired(), h.ServeMedia)

	key := "posts/user-1/550e8400-e29b-41d4-a716-446655440000.jpg"
	content := "fake-image-bytes"
	require.NoError(t, store.PutObject(t.Context(), key, strings.NewReader(content), int64(len(content)), "image/jpeg"))

	token := testAccessToken(t, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/media/file?key="+key, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Equal(t, "16", w.Header().Get("Content-Length"))
	assert.Equal(t, "public, max-age=86400", w.Header().Get("Cache-Control"))
	assert.Equal(t, content, w.Body.String())
}

func TestMediaServeMediaNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := storage.NewMemoryMediaStore("")
	h := &MediaHandler{Store: store}

	r := gin.New()
	r.GET("/media/file", auth.AuthRequired(), h.ServeMedia)

	token := testAccessToken(t, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/media/file?key=posts/user-1/550e8400-e29b-41d4-a716-446655440001.jpg", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func testAccessToken(t *testing.T, userID string) string {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-jwt-secret-for-media-tests")
	pair, err := auth.GenerateTokenPair(userID)
	require.NoError(t, err)
	return pair.AccessToken
}
