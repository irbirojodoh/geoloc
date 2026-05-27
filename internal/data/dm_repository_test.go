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

func applyDMMigration(t *testing.T) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "migrations", "007_dm.cql"))
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
		ConversationID: conv.ConversationID,
		SenderID:         a,
		Ciphertext:       base64.StdEncoding.EncodeToString(make([]byte, 17)),
		Nonce:            base64.StdEncoding.EncodeToString(make([]byte, 12)),
		KeyVersion:       1,
	}

	require.NoError(t, repo.SendMessage(ctx, msg))
	msgs, _, err := repo.ListMessages(ctx, conv.ConversationID, nil, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, a, msgs[0].SenderID)
}
