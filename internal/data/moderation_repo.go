package data

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gocql/gocql"
)

// ModerationRepository handles content moderation: reports, blocks, mutes
type ModerationRepository struct {
	session *gocql.Session
}

// NewModerationRepository creates a new moderation repository
func NewModerationRepository(session *gocql.Session) *ModerationRepository {
	return &ModerationRepository{session: session}
}

// Report reason constants
const (
	ReportReasonSpam          = "spam"
	ReportReasonHarassment    = "harassment"
	ReportReasonInappropriate = "inappropriate"
	ReportReasonOther         = "other"

	ReportStatusPending   = "pending"
	ReportStatusReviewed  = "reviewed"
	ReportStatusResolved  = "resolved"
	ReportStatusDismissed = "dismissed"
)

// ValidReportReasons lists all valid report reasons
var ValidReportReasons = map[string]bool{
	ReportReasonSpam:          true,
	ReportReasonHarassment:    true,
	ReportReasonInappropriate: true,
	ReportReasonOther:         true,
}

// ValidTargetTypes lists valid report target types
var ValidTargetTypes = map[string]bool{
	"post":    true,
	"comment": true,
	"user":    true,
}

// ============== REPORTS ==============

// CreateReport creates a new content report
func (r *ModerationRepository) CreateReport(ctx context.Context, reporterID, targetType, targetID, reason, description string) error {
	reporterUUID, err := gocql.ParseUUID(reporterID)
	if err != nil {
		return fmt.Errorf("invalid reporter_id: %w", err)
	}

	targetUUID, err := gocql.ParseUUID(targetID)
	if err != nil {
		return fmt.Errorf("invalid target_id: %w", err)
	}

	reportID := gocql.TimeUUID()
	now := time.Now()

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	// Insert into reports table
	batch.Query(`
		INSERT INTO reports (id, reporter_id, target_type, target_id, reason, description, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, reportID, reporterUUID, targetType, targetUUID, reason, description, ReportStatusPending, now)

	// Insert into reports_by_user for duplicate checking
	batch.Query(`
		INSERT INTO reports_by_user (reporter_id, target_type, target_id, report_id, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, reporterUUID, targetType, targetUUID, reportID, now)

	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}

	slog.Info("[MODERATION] Report created",
		"report_id", reportID.String(),
		"reporter_id", reporterID,
		"target_type", targetType,
		"target_id", targetID,
		"reason", reason,
	)

	return nil
}

// HasReported checks if a user has already reported a specific target
func (r *ModerationRepository) HasReported(ctx context.Context, reporterID, targetType, targetID string) (bool, error) {
	reporterUUID, err := gocql.ParseUUID(reporterID)
	if err != nil {
		return false, nil
	}

	targetUUID, err := gocql.ParseUUID(targetID)
	if err != nil {
		return false, nil
	}

	// Check reports table for existing report from this user on this target
	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM reports WHERE target_type = ? AND target_id = ?
	`, targetType, targetUUID).WithContext(ctx).Scan(&count)
	if err != nil {
		return false, err
	}

	// Also verify reporter via reports_by_user
	if count > 0 {
		iter := r.session.Query(`
			SELECT target_type, target_id FROM reports_by_user WHERE reporter_id = ?
		`, reporterUUID).WithContext(ctx).Iter()

		var tt, tid string
		for iter.Scan(&tt, &tid) {
			if tt == targetType && tid == targetID {
				iter.Close()
				return true, nil
			}
		}
		iter.Close()
	}

	return false, nil
}

// ============== BLOCKS ==============

// BlockUser blocks another user (bidirectional visibility removal)
func (r *ModerationRepository) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	blockerUUID, err := gocql.ParseUUID(blockerID)
	if err != nil {
		return fmt.Errorf("invalid blocker_id: %w", err)
	}

	blockedUUID, err := gocql.ParseUUID(blockedID)
	if err != nil {
		return fmt.Errorf("invalid blocked_id: %w", err)
	}

	if blockerID == blockedID {
		return fmt.Errorf("cannot block yourself")
	}

	now := time.Now()

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	// Insert into blocks
	batch.Query(`
		INSERT INTO blocks (blocker_id, blocked_id, created_at) VALUES (?, ?, ?)
	`, blockerUUID, blockedUUID, now)

	// Insert into blocked_by (reverse lookup)
	batch.Query(`
		INSERT INTO blocked_by (blocked_id, blocker_id, created_at) VALUES (?, ?, ?)
	`, blockedUUID, blockerUUID, now)

	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to block user: %w", err)
	}

	return nil
}

// UnblockUser removes a block relationship
func (r *ModerationRepository) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	blockerUUID, err := gocql.ParseUUID(blockerID)
	if err != nil {
		return fmt.Errorf("invalid blocker_id: %w", err)
	}

	blockedUUID, err := gocql.ParseUUID(blockedID)
	if err != nil {
		return fmt.Errorf("invalid blocked_id: %w", err)
	}

	batch := r.session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	batch.Query(`
		DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?
	`, blockerUUID, blockedUUID)

	batch.Query(`
		DELETE FROM blocked_by WHERE blocked_id = ? AND blocker_id = ?
	`, blockedUUID, blockerUUID)

	if err := r.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("failed to unblock user: %w", err)
	}

	return nil
}

// IsBlocked checks if there is a block relationship in either direction
func (r *ModerationRepository) IsBlocked(ctx context.Context, userA, userB string) (bool, error) {
	userAUUID, err := gocql.ParseUUID(userA)
	if err != nil {
		return false, nil
	}

	userBUUID, err := gocql.ParseUUID(userB)
	if err != nil {
		return false, nil
	}

	// Check if A blocked B
	var count int
	err = r.session.Query(`
		SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?
	`, userAUUID, userBUUID).WithContext(ctx).Scan(&count)
	if err == nil && count > 0 {
		return true, nil
	}

	// Check if B blocked A
	err = r.session.Query(`
		SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?
	`, userBUUID, userAUUID).WithContext(ctx).Scan(&count)
	if err == nil && count > 0 {
		return true, nil
	}

	return false, nil
}

// GetBlockedUsers returns list of user IDs blocked by the given user
func (r *ModerationRepository) GetBlockedUsers(ctx context.Context, userID string) ([]string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	iter := r.session.Query(`
		SELECT blocked_id FROM blocks WHERE blocker_id = ?
	`, uid).WithContext(ctx).Iter()

	var blockedUsers []string
	var blockedID gocql.UUID

	for iter.Scan(&blockedID) {
		blockedUsers = append(blockedUsers, blockedID.String())
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return blockedUsers, nil
}

// ============== MUTES ==============

// MuteUser mutes another user (hides from feed without blocking interaction)
func (r *ModerationRepository) MuteUser(ctx context.Context, muterID, mutedID string) error {
	muterUUID, err := gocql.ParseUUID(muterID)
	if err != nil {
		return fmt.Errorf("invalid muter_id: %w", err)
	}

	mutedUUID, err := gocql.ParseUUID(mutedID)
	if err != nil {
		return fmt.Errorf("invalid muted_id: %w", err)
	}

	if muterID == mutedID {
		return fmt.Errorf("cannot mute yourself")
	}

	now := time.Now()

	err = r.session.Query(`
		INSERT INTO mutes (muter_id, muted_id, created_at) VALUES (?, ?, ?)
	`, muterUUID, mutedUUID, now).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("failed to mute user: %w", err)
	}

	return nil
}

// UnmuteUser removes a mute relationship
func (r *ModerationRepository) UnmuteUser(ctx context.Context, muterID, mutedID string) error {
	muterUUID, err := gocql.ParseUUID(muterID)
	if err != nil {
		return fmt.Errorf("invalid muter_id: %w", err)
	}

	mutedUUID, err := gocql.ParseUUID(mutedID)
	if err != nil {
		return fmt.Errorf("invalid muted_id: %w", err)
	}

	err = r.session.Query(`
		DELETE FROM mutes WHERE muter_id = ? AND muted_id = ?
	`, muterUUID, mutedUUID).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("failed to unmute user: %w", err)
	}

	return nil
}

// GetMutedUsers returns list of user IDs muted by the given user
func (r *ModerationRepository) GetMutedUsers(ctx context.Context, userID string) ([]string, error) {
	uid, err := gocql.ParseUUID(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	iter := r.session.Query(`
		SELECT muted_id FROM mutes WHERE muter_id = ?
	`, uid).WithContext(ctx).Iter()

	var mutedUsers []string
	var mutedID gocql.UUID

	for iter.Scan(&mutedID) {
		mutedUsers = append(mutedUsers, mutedID.String())
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return mutedUsers, nil
}

// GetBlockedAndMutedUsers returns both blocked and muted user IDs for feed filtering
func (r *ModerationRepository) GetBlockedAndMutedUsers(ctx context.Context, userID string) (map[string]bool, error) {
	excluded := make(map[string]bool)

	blocked, err := r.GetBlockedUsers(ctx, userID)
	if err != nil {
		slog.Warn("Failed to fetch blocked users", "error", err, "user_id", userID)
	}
	for _, id := range blocked {
		excluded[id] = true
	}

	// Also add users who have blocked the current user (bidirectional)
	uid, err := gocql.ParseUUID(userID)
	if err == nil {
		iter := r.session.Query(`
			SELECT blocker_id FROM blocked_by WHERE blocked_id = ?
		`, uid).WithContext(ctx).Iter()

		var blockerID gocql.UUID
		for iter.Scan(&blockerID) {
			excluded[blockerID.String()] = true
		}
		iter.Close()
	}

	muted, err := r.GetMutedUsers(ctx, userID)
	if err != nil {
		slog.Warn("Failed to fetch muted users", "error", err, "user_id", userID)
	}
	for _, id := range muted {
		excluded[id] = true
	}

	return excluded, nil
}
