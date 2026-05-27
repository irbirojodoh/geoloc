package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/stretchr/testify/require"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/models"
)

func init() {
	_ = os.Setenv("JWT_SECRET", "test-jwt-secret-for-dm-handlers")
}

type fakeMod struct {
	blocked bool
	err     error
}

func (f *fakeMod) IsBlocked(ctx context.Context, userA, userB string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.blocked, nil
}

type fakeDMRepo struct {
	getPubFn  func(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error)
	getConvFn func(ctx context.Context, id gocql.UUID) (*models.DMConversation, error)
	sendErr   error
	lastSent  *models.DMMessage
	upsertErr error

	storedConv *models.DMConversation
	storedPub  *models.PublicKeyRecord
}

func (f *fakeDMRepo) UpsertPublicKey(ctx context.Context, userID gocql.UUID, version int, pubKey string) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.storedPub = &models.PublicKeyRecord{UserID: userID, KeyVersion: version, PublicKey: pubKey, CreatedAt: time.Now().UTC()}
	return nil
}

func (f *fakeDMRepo) GetPublicKey(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
	if f.getPubFn != nil {
		return f.getPubFn(ctx, userID)
	}
	return f.storedPub, nil
}

func (f *fakeDMRepo) GetConversation(ctx context.Context, conversationID gocql.UUID) (*models.DMConversation, error) {
	if f.getConvFn != nil {
		return f.getConvFn(ctx, conversationID)
	}
	if f.storedConv != nil && f.storedConv.ConversationID == conversationID {
		return f.storedConv, nil
	}
	return nil, data.ErrDMConversationNotFound
}

func (f *fakeDMRepo) GetOrCreateConversation(ctx context.Context, userA, userB gocql.UUID) (*models.DMConversation, error) {
	return f.storedConv, nil
}

func (f *fakeDMRepo) ListConversations(ctx context.Context, userID gocql.UUID, pageState []byte, limit int) ([]models.DMConversationSummary, []byte, error) {
	return nil, nil, nil
}

func (f *fakeDMRepo) SendMessage(ctx context.Context, msg *models.DMMessage) error {
	cp := *msg
	f.lastSent = &cp
	return f.sendErr
}

func (f *fakeDMRepo) ListMessages(ctx context.Context, conversationID gocql.UUID, pageState []byte, limit int) ([]models.DMMessage, []byte, error) {
	return nil, nil, nil
}

func (f *fakeDMRepo) SoftDeleteMessage(ctx context.Context, conversationID gocql.UUID, messageID gocql.UUID, requesterID gocql.UUID) error {
	return nil
}

func (f *fakeDMRepo) MarkRead(ctx context.Context, conversationID, userID gocql.UUID, lastReadID gocql.UUID) error {
	return nil
}

func (f *fakeDMRepo) GetReadReceipts(ctx context.Context, conversationID gocql.UUID) ([]models.ReadReceipt, error) {
	return nil, nil
}

func dmTestRouter(h *DMHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(auth.AuthRequired())
	RegisterDMRoutes(api, h)
	return r
}

func bearerToken(t *testing.T, userID string) string {
	t.Helper()
	tok, err := auth.GenerateTokenPair(userID)
	require.NoError(t, err)
	return tok.AccessToken
}

func TestDMHandler_PostMessage_Valid(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{
		storedConv: conv,
		getPubFn: func(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
			if userID == other {
				return &models.PublicKeyRecord{KeyVersion: 3, PublicKey: "abc"}, nil
			}
			return nil, nil
		},
	}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)

	body := map[string]any{
		"ciphertext":  base64.StdEncoding.EncodeToString(make([]byte, 17)),
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 12)),
		"key_version": 3,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, self.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NotNil(t, repo.lastSent)
}

func TestDMHandler_PostMessage_KeyVersionMismatch(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{
		storedConv: conv,
		getPubFn: func(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
			return &models.PublicKeyRecord{KeyVersion: 9, PublicKey: "k"}, nil
		},
	}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)
	body := map[string]any{
		"ciphertext":  base64.StdEncoding.EncodeToString(make([]byte, 17)),
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 12)),
		"key_version": 1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, self.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "key_version_mismatch", resp["error"])
	require.Equal(t, float64(9), resp["current_version"])
}

func TestDMHandler_PostMessage_NotParticipant(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	third := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{storedConv: conv}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)
	body := map[string]any{
		"ciphertext":  base64.StdEncoding.EncodeToString(make([]byte, 17)),
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 12)),
		"key_version": 1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, third.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestDMHandler_PostMessage_Blocked(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{
		storedConv: conv,
		getPubFn: func(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
			return &models.PublicKeyRecord{KeyVersion: 1, PublicKey: "k"}, nil
		},
	}
	h := &DMHandler{DM: repo, Mod: &fakeMod{blocked: true}}
	r := dmTestRouter(h)
	body := map[string]any{
		"ciphertext":  base64.StdEncoding.EncodeToString(make([]byte, 17)),
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 12)),
		"key_version": 1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, self.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "blocked", resp["error"])
}

func TestDMHandler_PostMessage_InvalidBase64(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{storedConv: conv}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)
	body := map[string]any{
		"ciphertext":  "@@@",
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 12)),
		"key_version": 1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, self.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDMHandler_PostMessage_InvalidNonceLength(t *testing.T) {
	self := gocql.TimeUUID()
	other := gocql.TimeUUID()
	now := time.Now().UTC()
	conv := &models.DMConversation{
		ConversationID: gocql.TimeUUID(),
		ParticipantA:   self,
		ParticipantB:   other,
		CreatedAt:      now,
		LastMessageAt:  now,
	}
	repo := &fakeDMRepo{storedConv: conv}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)
	body := map[string]any{
		"ciphertext":  base64.StdEncoding.EncodeToString(make([]byte, 17)),
		"nonce":       base64.StdEncoding.EncodeToString(make([]byte, 8)),
		"key_version": 1,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/conversations/"+conv.ConversationID.String()+"/messages", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, self.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDMHandler_PublicKeyRoundTrip(t *testing.T) {
	uid := gocql.TimeUUID()
	repo := &fakeDMRepo{}
	h := &DMHandler{DM: repo, Mod: &fakeMod{}}
	r := dmTestRouter(h)

	putBody := map[string]any{"public_key": "cHVibGljLWtleS1kYXRh", "key_version": 2}
	b, _ := json.Marshal(putBody)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dm/keys", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+bearerToken(t, uid.String()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	repo.getPubFn = func(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
		if userID == uid {
			return repo.storedPub, nil
		}
		return nil, nil
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dm/keys/"+uid.String(), nil)
	req2.Header.Set("Authorization", "Bearer "+bearerToken(t, gocql.TimeUUID().String()))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &got))
	require.Equal(t, float64(2), got["key_version"])
	require.Equal(t, "cHVibGljLWtleS1kYXRh", got["public_key"])
}
