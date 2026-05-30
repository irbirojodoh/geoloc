package data

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/require"
	"social-geo-go/internal/models"
)

func applyCQLMigrationFile(t *testing.T, filename string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "migrations", filename))
	require.NoError(t, err)
	lines := strings.Split(string(b), "\n")
	var buf strings.Builder
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(trim), "USE ") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	for _, stmt := range strings.Split(buf.String(), ";") {
		q := strings.TrimSpace(stmt)
		if q == "" {
			continue
		}
		require.NoError(t, testSession.Query(q).Exec(), "statement: %s", q)
	}
}

func applyDMMigration(t *testing.T) {
	t.Helper()
	applyCQLMigrationFile(t, "007_dm.cql")
	applyCQLMigrationFile(t, "008_dm_multidevice.cql")
}

var dmMigrateOnce sync.Once

func ensureDMMigrated(t *testing.T) {
	t.Helper()
	dmMigrateOnce.Do(func() {
		applyDMMigration(t)
	})
}

func TestDMRepository_GetOrCreateAndSend(t *testing.T) {
	ensureDMMigrated(t)
	ctx := context.Background()
	repo := NewDMRepository(testSession)
	a := gocql.TimeUUID()
	b := gocql.TimeUUID()
	conv, err := repo.GetOrCreateConversation(ctx, a, b)
	require.NoError(t, err)
	require.LessOrEqual(t, conv.ParticipantA.String(), conv.ParticipantB.String())

	conv2, err := repo.GetOrCreateConversation(ctx, b, a)
	require.NoError(t, err)
	require.Equal(t, conv.ConversationID, conv2.ConversationID)

	msg := &models.DMMessage{
		ConversationID:   conv.ConversationID,
		SenderID:         a,
		Ciphertext:       base64.StdEncoding.EncodeToString(make([]byte, 17)),
		Nonce:            base64.StdEncoding.EncodeToString(make([]byte, 12)),
		KeyVersion:       1,
		SenderKeyVersion: 2,
	}

	require.NoError(t, repo.SendMessage(ctx, msg))
	msgs, _, err := repo.ListMessages(ctx, conv.ConversationID, nil, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, a, msgs[0].SenderID)
	require.Equal(t, 2, msgs[0].SenderKeyVersion)
}

func TestDMRepository_IdentityBackupAndKeyVersions(t *testing.T) {
	ensureDMMigrated(t)
	ctx := context.Background()
	repo := NewDMRepository(testSession)
	u := gocql.TimeUUID()

	require.NoError(t, repo.UpsertPublicKey(ctx, u, 1, "pk-v1"))
	require.NoError(t, repo.UpsertPublicKey(ctx, u, 2, "pk-v2"))

	versions, err := repo.ListPublicKeyVersions(ctx, u)
	require.NoError(t, err)
	require.Len(t, versions, 2)

	v1, err := repo.GetPublicKeyVersion(ctx, u, 1)
	require.NoError(t, err)
	require.NotNil(t, v1)
	require.Equal(t, "pk-v1", v1.PublicKey)

	backup := &models.DMIdentityBackup{
		UserID:        u,
		BackupVersion: 1,
		Ciphertext:    base64.StdEncoding.EncodeToString(make([]byte, 17)),
		Nonce:         base64.StdEncoding.EncodeToString(make([]byte, 12)),
		KdfSalt:       base64.StdEncoding.EncodeToString(make([]byte, 16)),
	}
	require.NoError(t, repo.PutIdentityBackup(ctx, backup))
	got, err := repo.GetIdentityBackup(ctx, u, 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, backup.Ciphertext, got.Ciphertext)
}
