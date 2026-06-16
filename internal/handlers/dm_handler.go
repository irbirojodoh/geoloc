package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"
	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/middleware"
	"social-geo-go/internal/models"
	"social-geo-go/internal/notifications/kafka"
	"social-geo-go/internal/cache"
)

const dmMaxPageSize = 100

// dmModeration is the subset of moderation behavior required by DM routes.
type dmModeration interface {
	IsBlocked(ctx context.Context, userA, userB string) (bool, error)
}

// DMHandler wires DM HTTP endpoints.
type DMHandler struct {
	DM                   data.DMRepository
	Mod                  dmModeration
	Redis                *redis.Client
	RedisLimiter         *cache.RedisClient
	DMKafka              *kafka.DMMessageProducer
	KafkaNotificationsOn bool
}

func assertParticipant(conv *models.DMConversation, claimed gocql.UUID) error {
	if conv.ParticipantA != claimed && conv.ParticipantB != claimed {
		return data.ErrDMNotParticipant
	}
	return nil
}

func otherParticipant(conv *models.DMConversation, self gocql.UUID) (gocql.UUID, error) {
	if conv.ParticipantA == self {
		return conv.ParticipantB, nil
	}
	if conv.ParticipantB == self {
		return conv.ParticipantA, nil
	}
	return gocql.UUID{}, data.ErrDMNotParticipant
}

func parseUUIDParam(c *gin.Context, name string) (gocql.UUID, bool) {
	s := strings.TrimSpace(c.Param(name))
	if s == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return gocql.UUID{}, false
	}
	u, err := gocql.ParseUUID(s)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return gocql.UUID{}, false
	}
	return u, true
}

func parseUUIDQuery(c *gin.Context, name string) (gocql.UUID, bool) {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_" + name})
		return gocql.UUID{}, false
	}
	u, err := gocql.ParseUUID(s)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return gocql.UUID{}, false
	}
	return u, true
}

func decodePageState(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func encodePageState(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func validateOpaqueCiphertext(ciphertextB64, nonceB64 string) error {
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return errors.New("invalid_ciphertext_base64")
	}
	if len(ct) < 17 {
		return errors.New("invalid_ciphertext_length")
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return errors.New("invalid_nonce_base64")
	}
	if len(nonce) != 12 {
		return errors.New("invalid_nonce_length")
	}
	return nil
}

func validateIdentityBackup(ciphertextB64, nonceB64, kdfSaltB64 string) error {
	if err := validateOpaqueCiphertext(ciphertextB64, nonceB64); err != nil {
		return err
	}
	salt, err := base64.StdEncoding.DecodeString(kdfSaltB64)
	if err != nil {
		return errors.New("invalid_kdf_salt_base64")
	}
	if len(salt) < 16 {
		return errors.New("invalid_kdf_salt_length")
	}
	return nil
}

func publicKeyResponse(rec *models.PublicKeyRecord) gin.H {
	return gin.H{
		"user_id":     rec.UserID.String(),
		"public_key":  rec.PublicKey,
		"key_version": rec.KeyVersion,
		"created_at":  rec.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (h *DMHandler) recipientOnline(ctx context.Context, userID string) bool {
	if h.Redis == nil {
		return false
	}
	n, err := h.Redis.Exists(ctx, "sse:online:"+userID).Result()
	return err == nil && n > 0
}

func (h *DMHandler) maybeProduceDMKafka(ctx context.Context, partitionKey string, payload any) {
	if h.DMKafka == nil {
		return
	}
	should := h.KafkaNotificationsOn || !h.recipientOnline(ctx, partitionKey)
	if !should {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("dm kafka marshal failed", "error", err)
		return
	}
	if err := h.DMKafka.Publish(ctx, partitionKey, raw); err != nil {
		slog.Warn("dm kafka publish failed", "recipient_id", partitionKey, "error", err)
	}
}

func (h *DMHandler) publishRedisDM(ctx context.Context, recipientUserID string, payload any) {
	if h.Redis == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("dm redis marshal failed", "error", err)
		return
	}
	ch := "dm:" + recipientUserID
	if err := h.Redis.Publish(ctx, ch, raw).Err(); err != nil {
		slog.Warn("dm redis publish failed", "channel", ch, "error", err)
	}
}

// RegisterDMRoutes registers /dm routes under the given authenticated group.
func RegisterDMRoutes(api *gin.RouterGroup, h *DMHandler) {
	if h == nil {
		return
	}
	dm := api.Group("/dm")
	{
		dm.GET("/keys/backup", h.getIdentityBackup)
		dm.GET("/key-backup", h.getIdentityBackup)
		dm.GET("/keys/:userID/versions", h.listPublicKeyVersions)
		dm.GET("/keys/:userID", h.getPublicKey)
		dm.GET("/conversations", h.listConversations)
		dm.GET("/conversations/:id/messages", h.listMessages)
	}

	write := api.Group("/dm")
	if h.RedisLimiter != nil {
		write.Use(middleware.RateLimitByUser(h.RedisLimiter, 60, time.Minute))
	}
	{
		write.PUT("/keys", h.putPublicKey)
		write.PUT("/keys/backup", h.putIdentityBackup)
		write.PUT("/key-backup", h.putIdentityBackup)
		write.POST("/conversations", h.postConversation)
		write.POST("/conversations/:id/messages", h.postMessage)
		write.DELETE("/conversations/:id", h.deleteConversation)
		write.DELETE("/messages/:messageID", h.deleteMessage)
		write.PUT("/conversations/:id/read", h.putRead)
	}
}

type putDMKeyRequest struct {
	PublicKey  string `json:"public_key"`
	KeyVersion int    `json:"key_version"`
}

func (h *DMHandler) putPublicKey(c *gin.Context) {
	uidStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(uidStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req putDMKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.PublicKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	if req.KeyVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_key_version"})
		return
	}
	if err := h.DM.UpsertPublicKey(c.Request.Context(), self, req.KeyVersion, strings.TrimSpace(req.PublicKey)); err != nil {
		slog.Error("dm upsert public key failed", "user_id", uidStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DMHandler) getPublicKey(c *gin.Context) {
	targetStr := strings.TrimSpace(c.Param("userID"))
	target, err := gocql.ParseUUID(targetStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return
	}
	ctx := c.Request.Context()
	var rec *models.PublicKeyRecord
	if vStr := strings.TrimSpace(c.Query("key_version")); vStr != "" {
		v, err := strconv.Atoi(vStr)
		if err != nil || v < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_key_version"})
			return
		}
		rec, err = h.DM.GetPublicKeyVersion(ctx, target, v)
	} else {
		rec, err = h.DM.GetPublicKey(ctx, target)
	}
	if err != nil {
		slog.Error("dm get public key failed", "target_user_id", targetStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if rec == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "public_key_not_found"})
		return
	}
	c.JSON(http.StatusOK, publicKeyResponse(rec))
}

func (h *DMHandler) listPublicKeyVersions(c *gin.Context) {
	targetStr := strings.TrimSpace(c.Param("userID"))
	target, err := gocql.ParseUUID(targetStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return
	}
	rows, err := h.DM.ListPublicKeyVersions(c.Request.Context(), target)
	if err != nil {
		slog.Error("dm list public key versions failed", "target_user_id", targetStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicKeyResponse(&r))
	}
	c.JSON(http.StatusOK, gin.H{"versions": out})
}

type putIdentityBackupRequest struct {
	BackupVersion int    `json:"backup_version"`
	Ciphertext    string `json:"ciphertext"`
	Nonce         string `json:"nonce"`
	KdfSalt       string `json:"kdf_salt"`
}

func (h *DMHandler) putIdentityBackup(c *gin.Context) {
	uidStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(uidStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req putIdentityBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	if req.BackupVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_backup_version"})
		return
	}
	if err := validateIdentityBackup(req.Ciphertext, req.Nonce, req.KdfSalt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	backup := &models.DMIdentityBackup{
		UserID:        self,
		BackupVersion: req.BackupVersion,
		Ciphertext:    req.Ciphertext,
		Nonce:         req.Nonce,
		KdfSalt:       req.KdfSalt,
	}
	if err := h.DM.PutIdentityBackup(c.Request.Context(), backup); err != nil {
		slog.Error("dm put identity backup failed", "user_id", uidStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DMHandler) getIdentityBackup(c *gin.Context) {
	uidStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(uidStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	backupVersion := 0
	if vStr := strings.TrimSpace(c.Query("backup_version")); vStr != "" {
		backupVersion, err = strconv.Atoi(vStr)
		if err != nil || backupVersion < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_backup_version"})
			return
		}
	}
	b, err := h.DM.GetIdentityBackup(c.Request.Context(), self, backupVersion)
	if err != nil {
		slog.Error("dm get identity backup failed", "user_id", uidStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if b == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup_not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"backup_version": b.BackupVersion,
		"ciphertext":     b.Ciphertext,
		"nonce":          b.Nonce,
		"kdf_salt":       b.KdfSalt,
		"updated_at":     b.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
}

type postConversationRequest struct {
	UserID string `json:"user_id"`
}

func (h *DMHandler) postConversation(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req postConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	other, err := gocql.ParseUUID(strings.TrimSpace(req.UserID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_uuid"})
		return
	}
	if other == self {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_peer"})
		return
	}
	blocked, err := h.Mod.IsBlocked(c.Request.Context(), selfStr, other.String())
	if err != nil {
		slog.Error("dm block check failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if blocked {
		c.JSON(http.StatusForbidden, gin.H{"error": "blocked"})
		return
	}
	conv, err := h.DM.GetOrCreateConversation(c.Request.Context(), self, other)
	if err != nil {
		slog.Error("dm get or create conversation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"conversation": gin.H{
			"conversation_id": conv.ConversationID.String(),
			"participant_a":   conv.ParticipantA.String(),
			"participant_b":   conv.ParticipantB.String(),
			"created_at":      conv.CreatedAt.UTC().Format(time.RFC3339Nano),
			"last_message_at": conv.LastMessageAt.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (h *DMHandler) deleteConversation(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	conv, err := h.DM.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm get conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if assertParticipant(conv, self) != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.DM.DeleteConversation(c.Request.Context(), convID, self); err != nil {
		if errors.Is(err, data.ErrDMNotParticipant) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm delete conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DMHandler) listConversations(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	}
	if limit > dmMaxPageSize {
		limit = dmMaxPageSize
	}
	ps, err := decodePageState(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_cursor"})
		return
	}
	rows, next, err := h.DM.ListConversations(c.Request.Context(), self, ps, limit)
	if err != nil {
		slog.Error("dm list conversations failed", "user_id", selfStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"conversation_id": r.ConversationID.String(),
			"other_user_id":   r.OtherUserID.String(),
			"last_message_at": r.LastMessageAt.UTC().Format(time.RFC3339Nano),
			"created_at":      r.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"conversations": out,
		"next_cursor":   encodePageState(next),
	})
}

func (h *DMHandler) listMessages(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	conv, err := h.DM.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm get conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if assertParticipant(conv, self) != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	if limit > dmMaxPageSize {
		limit = dmMaxPageSize
	}
	ps, err := decodePageState(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_cursor"})
		return
	}
	msgs, next, err := h.DM.ListMessages(c.Request.Context(), convID, ps, limit)
	if err != nil {
		slog.Error("dm list messages failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	out := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		item := gin.H{
			"message_id":         m.MessageID.String(),
			"sender_id":          m.SenderID.String(),
			"ciphertext":         m.Ciphertext,
			"nonce":              m.Nonce,
			"key_version":        m.KeyVersion,
			"sender_key_version": m.SenderKeyVersion,
			"sent_at":            m.SentAt.UTC().Format(time.RFC3339Nano),
		}
		if m.DeletedAt != nil {
			item["deleted_at"] = m.DeletedAt.UTC().Format(time.RFC3339Nano)
		} else {
			item["deleted_at"] = nil
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{
		"messages":    out,
		"next_cursor": encodePageState(next),
	})
}

type postMessageRequest struct {
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
	KeyVersion int    `json:"key_version"`
}

func (h *DMHandler) postMessage(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req postMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	if err := validateOpaqueCiphertext(req.Ciphertext, req.Nonce); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conv, err := h.DM.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm get conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if assertParticipant(conv, self) != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	recipient, err := otherParticipant(conv, self)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	blocked, err := h.Mod.IsBlocked(c.Request.Context(), selfStr, recipient.String())
	if err != nil {
		slog.Error("dm block check failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if blocked {
		c.JSON(http.StatusForbidden, gin.H{"error": "blocked"})
		return
	}
	pk, err := h.DM.GetPublicKey(c.Request.Context(), recipient)
	if err != nil {
		slog.Error("dm get recipient key failed", "recipient_id", recipient.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if pk == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "recipient_has_no_public_key"})
		return
	}
	if pk.KeyVersion != req.KeyVersion {
		c.JSON(http.StatusConflict, gin.H{
			"error":            "key_version_mismatch",
			"current_version":  pk.KeyVersion,
		})
		return
	}
	senderPK, err := h.DM.GetPublicKey(c.Request.Context(), self)
	if err != nil {
		slog.Error("dm get sender key failed", "sender_id", selfStr, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if senderPK == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "sender_has_no_public_key"})
		return
	}
	msg := &models.DMMessage{
		ConversationID:   convID,
		SenderID:         self,
		Ciphertext:       req.Ciphertext,
		Nonce:            req.Nonce,
		KeyVersion:       req.KeyVersion,
		SenderKeyVersion: senderPK.KeyVersion,
	}
	if err := h.DM.SendMessage(c.Request.Context(), msg); err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm send message failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	sentAt := msg.SentAt.UTC().Format(time.RFC3339Nano)
	evt := gin.H{
		"type":               "dm_new_message",
		"event":              "dm.message.created",
		"conversation_id":    convID.String(),
		"message_id":         msg.MessageID.String(),
		"sender_id":          self.String(),
		"ciphertext":         req.Ciphertext,
		"nonce":              req.Nonce,
		"key_version":        req.KeyVersion,
		"sender_key_version": senderPK.KeyVersion,
		"sent_at":            sentAt,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.publishRedisDM(ctx, recipient.String(), evt)
		h.maybeProduceDMKafka(ctx, recipient.String(), evt)
	}()

	c.JSON(http.StatusCreated, gin.H{
		"message_id": msg.MessageID.String(),
		"sent_at":    sentAt,
	})
}

func (h *DMHandler) deleteMessage(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	msgID, ok := parseUUIDParam(c, "messageID")
	if !ok {
		return
	}
	convID, ok := parseUUIDQuery(c, "conversation_id")
	if !ok {
		return
	}
	conv, err := h.DM.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm get conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if assertParticipant(conv, self) != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.DM.SoftDeleteMessage(c.Request.Context(), convID, msgID, self); err != nil {
		if errors.Is(err, data.ErrDMMessageNotFound) || errors.Is(err, data.ErrDMNotMessageOwner) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm soft delete failed", "conversation_id", convID.String(), "message_id", msgID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	c.Status(http.StatusNoContent)
}

type putReadRequest struct {
	LastReadID string `json:"last_read_id"`
}

func (h *DMHandler) putRead(c *gin.Context) {
	selfStr := auth.GetUserID(c)
	self, err := gocql.ParseUUID(selfStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	convID, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	var req putReadRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.LastReadID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	lastRead, err := gocql.ParseUUID(strings.TrimSpace(req.LastReadID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_last_read_id"})
		return
	}
	conv, err := h.DM.GetConversation(c.Request.Context(), convID)
	if err != nil {
		if errors.Is(err, data.ErrDMConversationNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		slog.Error("dm get conversation failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	if assertParticipant(conv, self) != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	other, err := otherParticipant(conv, self)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := h.DM.MarkRead(c.Request.Context(), convID, self, lastRead); err != nil {
		slog.Error("dm mark read failed", "conversation_id", convID.String(), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	readAt := time.Now().UTC().Format(time.RFC3339Nano)
	evt := gin.H{
		"type":            "dm_read_receipt",
		"conversation_id": convID.String(),
		"last_read_id":    lastRead.String(),
		"read_at":         readAt,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.publishRedisDM(ctx, other.String(), evt)
		if h.DMKafka != nil && h.KafkaNotificationsOn {
			raw, err := json.Marshal(evt)
			if err != nil {
				slog.Warn("dm read receipt kafka marshal failed", "error", err)
				return
			}
			if err := h.DMKafka.Publish(ctx, other.String(), raw); err != nil {
				slog.Warn("dm read receipt kafka publish failed", "error", err)
			}
		}
	}()
	c.Status(http.StatusNoContent)
}
