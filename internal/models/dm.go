package models

import (
	"time"

	"github.com/gocql/gocql"
)

// PublicKeyRecord is the server-side view of a user's uploaded X25519 public key.
type PublicKeyRecord struct {
	UserID     gocql.UUID `json:"user_id"`
	KeyVersion int        `json:"key_version"`
	PublicKey  string     `json:"public_key"`
	CreatedAt  time.Time  `json:"created_at"`
}

// DMConversation is a two-party conversation row.
type DMConversation struct {
	ConversationID  gocql.UUID `json:"conversation_id"`
	ParticipantA    gocql.UUID `json:"participant_a"`
	ParticipantB    gocql.UUID `json:"participant_b"`
	CreatedAt       time.Time  `json:"created_at"`
	LastMessageAt   time.Time  `json:"last_message_at"`
}

// DMConversationSummary is a row in the user's conversation list.
type DMConversationSummary struct {
	ConversationID gocql.UUID `json:"conversation_id"`
	OtherUserID    gocql.UUID `json:"other_user_id"`
	LastMessageAt  time.Time  `json:"last_message_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// DMMessage is a single encrypted message (opaque ciphertext to the server).
type DMMessage struct {
	ConversationID   gocql.UUID
	MessageID        gocql.UUID
	SenderID         gocql.UUID
	Ciphertext       string
	Nonce            string
	KeyVersion       int // recipient public key version used for ECDH
	SenderKeyVersion int // sender public key version at encrypt time
	SentAt           time.Time
	DeletedAt        *time.Time
}

// DMIdentityBackup is a passphrase-encrypted identity key bundle (opaque to server).
type DMIdentityBackup struct {
	UserID         gocql.UUID
	BackupVersion  int       `json:"backup_version"`
	Ciphertext     string    `json:"ciphertext"`
	Nonce          string    `json:"nonce"`
	KdfSalt        string    `json:"kdf_salt"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ReadReceipt is the last read pointer for a user in a conversation.
type ReadReceipt struct {
	UserID     gocql.UUID `json:"user_id"`
	LastReadID gocql.UUID `json:"last_read_id"`
	ReadAt     time.Time  `json:"read_at"`
}
