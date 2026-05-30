package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"social-geo-go/internal/models"
)

// Deterministic namespace for UUIDv5-style conversation IDs (SHA-1 name-based UUID).
var dmConversationNamespace = uuid.MustParse("6f9619ff-8b86-d011-b42d-00cf4fc964ff")

var (
	// ErrDMConversationNotFound is returned when a conversation row is missing.
	ErrDMConversationNotFound = errors.New("dm conversation not found")
	// ErrDMNotParticipant is returned when a user is not part of a conversation.
	ErrDMNotParticipant = errors.New("dm not a participant")
	// ErrDMMessageNotFound is returned when a message row is missing.
	ErrDMMessageNotFound = errors.New("dm message not found")
	// ErrDMNotMessageOwner is returned when a user tries to delete another user's message.
	ErrDMNotMessageOwner = errors.New("dm not message owner")
)

// DMRepository persists DM metadata and ciphertext.
type DMRepository interface {
	UpsertPublicKey(ctx context.Context, userID gocql.UUID, version int, pubKey string) error
	GetPublicKey(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error)
	GetPublicKeyVersion(ctx context.Context, userID gocql.UUID, version int) (*models.PublicKeyRecord, error)
	ListPublicKeyVersions(ctx context.Context, userID gocql.UUID) ([]models.PublicKeyRecord, error)

	PutIdentityBackup(ctx context.Context, backup *models.DMIdentityBackup) error
	GetIdentityBackup(ctx context.Context, userID gocql.UUID, backupVersion int) (*models.DMIdentityBackup, error)

	GetConversation(ctx context.Context, conversationID gocql.UUID) (*models.DMConversation, error)
	GetOrCreateConversation(ctx context.Context, userA, userB gocql.UUID) (*models.DMConversation, error)
	DeleteConversation(ctx context.Context, conversationID, userID gocql.UUID) error
	ListConversations(ctx context.Context, userID gocql.UUID, pageState []byte, limit int) ([]models.DMConversationSummary, []byte, error)

	SendMessage(ctx context.Context, msg *models.DMMessage) error
	ListMessages(ctx context.Context, conversationID gocql.UUID, pageState []byte, limit int) ([]models.DMMessage, []byte, error)
	SoftDeleteMessage(ctx context.Context, conversationID gocql.UUID, messageID gocql.UUID, requesterID gocql.UUID) error

	MarkRead(ctx context.Context, conversationID, userID gocql.UUID, lastReadID gocql.UUID) error
	GetReadReceipts(ctx context.Context, conversationID gocql.UUID) ([]models.ReadReceipt, error)
}

type dmRepository struct {
	session *gocql.Session
}

// NewDMRepository creates a Cassandra-backed DMRepository.
func NewDMRepository(session *gocql.Session) DMRepository {
	return &dmRepository{session: session}
}

func sortParticipantUUIDs(a, b gocql.UUID) (smaller, larger gocql.UUID) {
	if strings.Compare(a.String(), b.String()) <= 0 {
		return a, b
	}
	return b, a
}

func conversationIDForParticipants(a, b gocql.UUID) (gocql.UUID, error) {
	pa, pb := sortParticipantUUIDs(a, b)
	name := pa.String() + "," + pb.String()
	gu := uuid.NewSHA1(dmConversationNamespace, []byte(name))
	return gocql.ParseUUID(gu.String())
}

// UpsertPublicKey inserts or replaces a key row; identical (user, version, key) is a no-op.
func (r *dmRepository) UpsertPublicKey(ctx context.Context, userID gocql.UUID, version int, pubKey string) error {
	var existing string
	err := r.session.Query(`
		SELECT public_key FROM user_public_keys WHERE user_id = ? AND key_version = ?
	`, userID, version).WithContext(ctx).Scan(&existing)
	if err == nil && existing == pubKey {
		return nil
	}
	if err != nil && err != gocql.ErrNotFound {
		return err
	}

	now := time.Now().UTC()
	return r.session.Query(`
		INSERT INTO user_public_keys (user_id, key_version, public_key, created_at)
		VALUES (?, ?, ?, ?)
	`, userID, version, pubKey, now).WithContext(ctx).Exec()
}

// GetPublicKey returns the newest key version for the user.
func (r *dmRepository) GetPublicKey(ctx context.Context, userID gocql.UUID) (*models.PublicKeyRecord, error) {
	var rec models.PublicKeyRecord
	err := r.session.Query(`
		SELECT user_id, key_version, public_key, created_at
		FROM user_public_keys WHERE user_id = ? LIMIT 1
	`, userID).WithContext(ctx).Scan(&rec.UserID, &rec.KeyVersion, &rec.PublicKey, &rec.CreatedAt)
	if err == gocql.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// GetPublicKeyVersion returns a specific public key version for a user.
func (r *dmRepository) GetPublicKeyVersion(ctx context.Context, userID gocql.UUID, version int) (*models.PublicKeyRecord, error) {
	var rec models.PublicKeyRecord
	err := r.session.Query(`
		SELECT user_id, key_version, public_key, created_at
		FROM user_public_keys WHERE user_id = ? AND key_version = ?
	`, userID, version).WithContext(ctx).Scan(&rec.UserID, &rec.KeyVersion, &rec.PublicKey, &rec.CreatedAt)
	if err == gocql.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListPublicKeyVersions returns all uploaded public key versions for a user (newest first).
func (r *dmRepository) ListPublicKeyVersions(ctx context.Context, userID gocql.UUID) ([]models.PublicKeyRecord, error) {
	iter := r.session.Query(`
		SELECT user_id, key_version, public_key, created_at
		FROM user_public_keys WHERE user_id = ?
	`, userID).WithContext(ctx).Iter()

	var out []models.PublicKeyRecord
	var rec models.PublicKeyRecord
	for iter.Scan(&rec.UserID, &rec.KeyVersion, &rec.PublicKey, &rec.CreatedAt) {
		out = append(out, rec)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// PutIdentityBackup stores or replaces an encrypted identity backup row.
func (r *dmRepository) PutIdentityBackup(ctx context.Context, backup *models.DMIdentityBackup) error {
	if backup.UpdatedAt.IsZero() {
		backup.UpdatedAt = time.Now().UTC()
	}
	var existingCT string
	err := r.session.Query(`
		SELECT ciphertext FROM user_dm_identity_backups WHERE user_id = ? AND backup_version = ?
	`, backup.UserID, backup.BackupVersion).WithContext(ctx).Scan(&existingCT)
	if err == nil && existingCT == backup.Ciphertext {
		return nil
	}
	if err != nil && err != gocql.ErrNotFound {
		return err
	}
	return r.session.Query(`
		INSERT INTO user_dm_identity_backups (user_id, backup_version, ciphertext, nonce, kdf_salt, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, backup.UserID, backup.BackupVersion, backup.Ciphertext, backup.Nonce, backup.KdfSalt, backup.UpdatedAt).WithContext(ctx).Exec()
}

// GetIdentityBackup loads a backup; backupVersion 0 means latest.
func (r *dmRepository) GetIdentityBackup(ctx context.Context, userID gocql.UUID, backupVersion int) (*models.DMIdentityBackup, error) {
	var b models.DMIdentityBackup
	var err error
	if backupVersion > 0 {
		err = r.session.Query(`
			SELECT user_id, backup_version, ciphertext, nonce, kdf_salt, updated_at
			FROM user_dm_identity_backups WHERE user_id = ? AND backup_version = ?
		`, userID, backupVersion).WithContext(ctx).Scan(
			&b.UserID, &b.BackupVersion, &b.Ciphertext, &b.Nonce, &b.KdfSalt, &b.UpdatedAt,
		)
	} else {
		err = r.session.Query(`
			SELECT user_id, backup_version, ciphertext, nonce, kdf_salt, updated_at
			FROM user_dm_identity_backups WHERE user_id = ? LIMIT 1
		`, userID).WithContext(ctx).Scan(
			&b.UserID, &b.BackupVersion, &b.Ciphertext, &b.Nonce, &b.KdfSalt, &b.UpdatedAt,
		)
	}
	if err == gocql.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// GetConversation loads a conversation by id.
func (r *dmRepository) GetConversation(ctx context.Context, conversationID gocql.UUID) (*models.DMConversation, error) {
	var c models.DMConversation
	c.ConversationID = conversationID
	err := r.session.Query(`
		SELECT participant_a, participant_b, created_at, last_message_at
		FROM dm_conversations WHERE conversation_id = ?
	`, conversationID).WithContext(ctx).Scan(&c.ParticipantA, &c.ParticipantB, &c.CreatedAt, &c.LastMessageAt)
	if err == gocql.ErrNotFound {
		return nil, ErrDMConversationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetOrCreateConversation returns the canonical conversation for two users.
func (r *dmRepository) GetOrCreateConversation(ctx context.Context, userA, userB gocql.UUID) (*models.DMConversation, error) {
	if userA == userB {
		return nil, fmt.Errorf("invalid self-conversation")
	}
	pa, pb := sortParticipantUUIDs(userA, userB)
	convID, err := conversationIDForParticipants(userA, userB)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	cas := make(map[string]any)
	applied, err := r.session.Query(`
		INSERT INTO dm_conversations (conversation_id, participant_a, participant_b, created_at, last_message_at)
		VALUES (?, ?, ?, ?, ?)
		IF NOT EXISTS
	`, convID, pa, pb, now, now).WithContext(ctx).MapScanCAS(cas)
	if err != nil {
		return nil, err
	}

	if applied {
		batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
		batch.Query(`
			INSERT INTO dm_conversations_by_user (user_id, last_message_at, conversation_id, other_user_id)
			VALUES (?, ?, ?, ?)
		`, pa, now, convID, pb)
		batch.Query(`
			INSERT INTO dm_conversations_by_user (user_id, last_message_at, conversation_id, other_user_id)
			VALUES (?, ?, ?, ?)
		`, pb, now, convID, pa)
		if err := r.session.ExecuteBatch(batch); err != nil {
			return nil, err
		}
		return &models.DMConversation{
			ConversationID: convID,
			ParticipantA:   pa,
			ParticipantB:   pb,
			CreatedAt:      now,
			LastMessageAt:  now,
		}, nil
	}

	existing, err := r.GetConversation(ctx, convID)
	if err != nil {
		return nil, err
	}
	if existing.ParticipantA != pa || existing.ParticipantB != pb {
		return nil, fmt.Errorf("dm conversation participant mismatch")
	}
	if err := r.ensureInboxRow(ctx, userA, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func otherUserID(conv *models.DMConversation, self gocql.UUID) gocql.UUID {
	if conv.ParticipantA == self {
		return conv.ParticipantB
	}
	return conv.ParticipantA
}

// ensureInboxRow puts the conversation back in a user's inbox (idempotent).
func (r *dmRepository) ensureInboxRow(ctx context.Context, userID gocql.UUID, conv *models.DMConversation) error {
	return r.session.Query(`
		INSERT INTO dm_conversations_by_user (user_id, last_message_at, conversation_id, other_user_id)
		VALUES (?, ?, ?, ?)
	`, userID, conv.LastMessageAt, conv.ConversationID, otherUserID(conv, userID)).WithContext(ctx).Exec()
}

// DeleteConversation removes the conversation from the requester's inbox only.
// Messages and the canonical conversation row are kept for the other participant.
// A new message or GetOrCreateConversation restores the requester's inbox row.
func (r *dmRepository) DeleteConversation(ctx context.Context, conversationID, userID gocql.UUID) error {
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if conv.ParticipantA != userID && conv.ParticipantB != userID {
		return ErrDMNotParticipant
	}
	return r.session.Query(`
		DELETE FROM dm_conversations_by_user
		WHERE user_id = ? AND last_message_at = ? AND conversation_id = ?
	`, userID, conv.LastMessageAt, conversationID).WithContext(ctx).Exec()
}

// ListConversations returns inbox rows newest-first with Cassandra paging.
func (r *dmRepository) ListConversations(ctx context.Context, userID gocql.UUID, pageState []byte, limit int) ([]models.DMConversationSummary, []byte, error) {
	if limit <= 0 {
		limit = 20
	}
	q := r.session.Query(`
		SELECT last_message_at, conversation_id, other_user_id
		FROM dm_conversations_by_user WHERE user_id = ?
	`, userID).WithContext(ctx).PageSize(limit).PageState(pageState)

	iter := q.Iter()
	var out []models.DMConversationSummary
	var lastMsgAt time.Time
	var convID gocql.UUID
	var other gocql.UUID
	for iter.Scan(&lastMsgAt, &convID, &other) {
		sum := models.DMConversationSummary{
			ConversationID: convID,
			OtherUserID:    other,
			LastMessageAt:  lastMsgAt,
		}
		out = append(out, sum)
	}
	next := iter.PageState()
	if err := iter.Close(); err != nil {
		return nil, nil, err
	}

	if len(out) == 0 {
		return out, next, nil
	}

	ids := make([]gocql.UUID, 0, len(out))
	for _, s := range out {
		ids = append(ids, s.ConversationID)
	}

	createdByID := make(map[string]time.Time, len(ids))
	for _, id := range ids {
		var createdAt time.Time
		err := r.session.Query(`
			SELECT created_at FROM dm_conversations WHERE conversation_id = ?
		`, id).WithContext(ctx).Scan(&createdAt)
		if err != nil {
			return nil, nil, err
		}
		createdByID[id.String()] = createdAt
	}
	for i := range out {
		out[i].CreatedAt = createdByID[out[i].ConversationID.String()]
	}

	return out, next, nil
}

// SendMessage appends a message and bumps inbox ordering for both participants.
func (r *dmRepository) SendMessage(ctx context.Context, msg *models.DMMessage) error {
	if msg.MessageID == (gocql.UUID{}) {
		msg.MessageID = gocql.TimeUUID()
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now().UTC()
	}

	var pa, pb gocql.UUID
	var oldLast time.Time
	err := r.session.Query(`
		SELECT participant_a, participant_b, last_message_at
		FROM dm_conversations WHERE conversation_id = ?
	`, msg.ConversationID).WithContext(ctx).Scan(&pa, &pb, &oldLast)
	if err == gocql.ErrNotFound {
		return ErrDMConversationNotFound
	}
	if err != nil {
		return err
	}

	newLast := msg.SentAt

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	batch.Query(`
		INSERT INTO dm_messages (conversation_id, message_id, sender_id, ciphertext, nonce, key_version, sender_key_version, sent_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ConversationID, msg.MessageID, msg.SenderID, msg.Ciphertext, msg.Nonce, msg.KeyVersion, msg.SenderKeyVersion, msg.SentAt, nil)

	batch.Query(`
		UPDATE dm_conversations SET last_message_at = ? WHERE conversation_id = ?
	`, newLast, msg.ConversationID)

	batch.Query(`
		DELETE FROM dm_conversations_by_user WHERE user_id = ? AND last_message_at = ? AND conversation_id = ?
	`, pa, oldLast, msg.ConversationID)
	batch.Query(`
		DELETE FROM dm_conversations_by_user WHERE user_id = ? AND last_message_at = ? AND conversation_id = ?
	`, pb, oldLast, msg.ConversationID)

	batch.Query(`
		INSERT INTO dm_conversations_by_user (user_id, last_message_at, conversation_id, other_user_id)
		VALUES (?, ?, ?, ?)
	`, pa, newLast, msg.ConversationID, pb)
	batch.Query(`
		INSERT INTO dm_conversations_by_user (user_id, last_message_at, conversation_id, other_user_id)
		VALUES (?, ?, ?, ?)
	`, pb, newLast, msg.ConversationID, pa)

	return r.session.ExecuteBatch(batch)
}

// ListMessages returns ciphertext rows newest-first.
func (r *dmRepository) ListMessages(ctx context.Context, conversationID gocql.UUID, pageState []byte, limit int) ([]models.DMMessage, []byte, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.session.Query(`
		SELECT message_id, sender_id, ciphertext, nonce, key_version, sender_key_version, sent_at, deleted_at
		FROM dm_messages WHERE conversation_id = ?
	`, conversationID).WithContext(ctx).PageSize(limit).PageState(pageState)

	iter := q.Iter()
	var out []models.DMMessage
	var mid gocql.UUID
	var sender gocql.UUID
	var ct, nonce string
	var kv int
	var senderKV int
	var sentAt time.Time
	var deletedAt *time.Time
	for iter.Scan(&mid, &sender, &ct, &nonce, &kv, &senderKV, &sentAt, &deletedAt) {
		out = append(out, models.DMMessage{
			ConversationID:   conversationID,
			MessageID:        mid,
			SenderID:         sender,
			Ciphertext:       ct,
			Nonce:            nonce,
			KeyVersion:       kv,
			SenderKeyVersion: senderKV,
			SentAt:           sentAt,
			DeletedAt:        deletedAt,
		})
	}
	next := iter.PageState()
	if err := iter.Close(); err != nil {
		return nil, nil, err
	}
	return out, next, nil
}

// SoftDeleteMessage tombstones a message; only the original sender may delete.
func (r *dmRepository) SoftDeleteMessage(ctx context.Context, conversationID gocql.UUID, messageID gocql.UUID, requesterID gocql.UUID) error {
	var sender gocql.UUID
	var deletedAt *time.Time
	err := r.session.Query(`
		SELECT sender_id, deleted_at FROM dm_messages WHERE conversation_id = ? AND message_id = ?
	`, conversationID, messageID).WithContext(ctx).Scan(&sender, &deletedAt)
	if err == gocql.ErrNotFound {
		return ErrDMMessageNotFound
	}
	if err != nil {
		return err
	}
	if sender != requesterID {
		return ErrDMNotMessageOwner
	}
	if deletedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	return r.session.Query(`
		UPDATE dm_messages SET deleted_at = ? WHERE conversation_id = ? AND message_id = ?
	`, now, conversationID, messageID).WithContext(ctx).Exec()
}

// MarkRead upserts a read receipt for the user in the conversation.
func (r *dmRepository) MarkRead(ctx context.Context, conversationID, userID gocql.UUID, lastReadID gocql.UUID) error {
	now := time.Now().UTC()
	return r.session.Query(`
		INSERT INTO dm_read_receipts (conversation_id, user_id, last_read_id, read_at)
		VALUES (?, ?, ?, ?)
	`, conversationID, userID, lastReadID, now).WithContext(ctx).Exec()
}

// GetReadReceipts returns all read pointers in a conversation.
func (r *dmRepository) GetReadReceipts(ctx context.Context, conversationID gocql.UUID) ([]models.ReadReceipt, error) {
	iter := r.session.Query(`
		SELECT user_id, last_read_id, read_at FROM dm_read_receipts WHERE conversation_id = ?
	`, conversationID).WithContext(ctx).Iter()

	var out []models.ReadReceipt
	var uid gocql.UUID
	var last gocql.UUID
	var readAt time.Time
	for iter.Scan(&uid, &last, &readAt) {
		out = append(out, models.ReadReceipt{UserID: uid, LastReadID: last, ReadAt: readAt})
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return out, nil
}
