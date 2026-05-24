package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"social-geo-go/internal/cache"
)

type CommentRepository struct {
	session        *gocql.Session
	commentCounter *cache.CommentCounter
}

func NewCommentRepository(session *gocql.Session, commentCounter *cache.CommentCounter) *CommentRepository {
	return &CommentRepository{session: session, commentCounter: commentCounter}
}

// CreateComment creates a new comment on a post or reply to another comment
func (r *CommentRepository) CreateComment(ctx context.Context, req *CreateCommentRequest) (*Comment, error) {
	commentID := gocql.TimeUUID()
	postID, err := gocql.ParseUUID(req.PostID)
	if err != nil {
		return nil, fmt.Errorf("invalid post_id: %w", err)
	}

	userID, err := gocql.ParseUUID(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	var parentID gocql.UUID
	depth := 1

	// If replying to a comment, validate parent and set depth
	if req.ParentID != "" {
		parentID, err = gocql.ParseUUID(req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_id: %w", err)
		}

		// Get parent comment depth
		parentComment, err := r.GetCommentByID(ctx, req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent comment not found: %w", err)
		}

		if parentComment.IsDeleted {
			return nil, fmt.Errorf("cannot reply to a deleted comment")
		}

		depth = parentComment.Depth + 1
		if depth > MaxCommentDepth {
			return nil, fmt.Errorf("maximum comment depth of %d reached", MaxCommentDepth)
		}
	}

	now := time.Now()

	batch := r.session.NewBatch(gocql.LoggedBatch)
	batch.WithContext(ctx)

	// Insert into comments table
	batch.Query(`
		INSERT INTO comments (post_id, comment_id, parent_id, user_id, content, depth, ip_address, user_agent, created_at, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, postID, commentID, parentID, userID, req.Content, depth, req.IPAddress, req.UserAgent, now, false)

	// Insert into comments_by_id for direct lookups
	batch.Query(`
		INSERT INTO comments_by_id (comment_id, post_id, parent_id, user_id, content, depth, ip_address, user_agent, created_at, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, commentID, postID, parentID, userID, req.Content, depth, req.IPAddress, req.UserAgent, now, false)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("ERROR: failed to create comment: %w", err)
	}

	// Increment comment count for post
	if r.commentCounter != nil {
		_, err = r.commentCounter.IncrementCommentCount(ctx, req.PostID)
		if err != nil {
			fmt.Printf("Warning: failed to increment Redis comment count: %v\n", err)
			// fallback
			err = r.session.Query(`
                UPDATE comment_counts SET count = count + 1
                WHERE post_id = ?
            `, postID).WithContext(ctx).Exec()
			if err != nil {
				fmt.Printf("Warning: failed to update comment count: %v\n", err)
			}
		}
	} else {
		err = r.session.Query(`
            UPDATE comment_counts SET count = count + 1
            WHERE post_id = ?
        `, postID).WithContext(ctx).Exec()
		if err != nil {
			fmt.Printf("Warning: failed to update comment count: %v\n", err)
		}
	}

	parentIDStr := ""
	if req.ParentID != "" {
		parentIDStr = parentID.String()
	}

	return &Comment{
		ID:        commentID.String(),
		PostID:    postID.String(),
		ParentID:  parentIDStr,
		UserID:    userID.String(),
		Content:   req.Content,
		Depth:     depth,
		CreatedAt: now,
		IsDeleted: false,
	}, nil
}

// EditComment updates the content of an existing comment
func (r *CommentRepository) EditComment(ctx context.Context, commentIDStr, userIDStr, content string) (*Comment, error) {
	comment, err := r.GetCommentByID(ctx, commentIDStr)
	if err != nil {
		return nil, err
	}

	if comment.UserID != userIDStr {
		return nil, fmt.Errorf("unauthorized: can only edit your own comments")
	}

	if comment.IsDeleted {
		return nil, fmt.Errorf("cannot edit a deleted comment")
	}

	cid, _ := gocql.ParseUUID(commentIDStr)
	pid, _ := gocql.ParseUUID(comment.PostID)
	now := time.Now()

	batch := r.session.NewBatch(gocql.LoggedBatch)
	batch.WithContext(ctx)

	// Update comments_by_id
	batch.Query(`
		UPDATE comments_by_id SET content = ?, updated_at = ? WHERE comment_id = ?
	`, content, now, cid)

	// Update comments
	batch.Query(`
		UPDATE comments SET content = ?, updated_at = ? WHERE post_id = ? AND created_at = ? AND comment_id = ?
	`, content, now, pid, comment.CreatedAt, cid)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to edit comment: %w", err)
	}

	comment.Content = content
	comment.UpdatedAt = &now
	return comment, nil
}

// GetCommentByID retrieves a comment by its ID
func (r *CommentRepository) GetCommentByID(ctx context.Context, id string) (*Comment, error) {
	commentID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid comment_id: %w", err)
	}

	var comment Comment
	var postID, parentID, userID gocql.UUID
	var updatedAt time.Time
	var isDeleted bool

	err = r.session.Query(`
		SELECT comment_id, post_id, parent_id, user_id, content, depth, created_at, updated_at, is_deleted
		FROM comments_by_id
		WHERE comment_id = ?
	`, commentID).WithContext(ctx).Scan(
		&commentID, &postID, &parentID, &userID, &comment.Content, &comment.Depth, &comment.CreatedAt, &updatedAt, &isDeleted,
	)

	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("comment not found")
		}
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}

	comment.ID = commentID.String()
	comment.PostID = postID.String()
	comment.UserID = userID.String()
	comment.IsDeleted = isDeleted
	if !updatedAt.IsZero() {
		comment.UpdatedAt = &updatedAt
	}
	if parentID.String() != "00000000-0000-0000-0000-000000000000" {
		comment.ParentID = parentID.String()
	}

	return &comment, nil
}

// GetCommentsForPost retrieves all comments for a post with nested structure (legacy)
func (r *CommentRepository) GetCommentsForPost(ctx context.Context, postID string, limit int) ([]Comment, error) {
	comments, _, _, err := r.GetCommentsForPostPaginated(ctx, postID, limit, "")
	return comments, err
}

// GetCommentsForPostPaginated retrieves top-level comments with cursor pagination
func (r *CommentRepository) GetCommentsForPostPaginated(ctx context.Context, postID string, limit int, cursor string) ([]Comment, string, bool, error) {
	pid, err := gocql.ParseUUID(postID)
	if err != nil {
		return nil, "", false, fmt.Errorf("invalid post_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	cursorTime, err := DecodeCursor(cursor)
	if err != nil {
		return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
	}

	// We fetch a bit more initially to account for some comments being replies
	// In Cassandra, ALLOW FILTERING is generally OK within a single partition
	var iter *gocql.Iter
	fetchLimit := limit * 3

	if cursorTime.IsZero() {
		iter = r.session.Query(`
			SELECT comment_id, parent_id, user_id, content, depth, created_at, updated_at, is_deleted
			FROM comments
			WHERE post_id = ? AND depth = 1 ALLOW FILTERING
		`, pid).WithContext(ctx).PageSize(fetchLimit).Iter()
	} else {
		iter = r.session.Query(`
			SELECT comment_id, parent_id, user_id, content, depth, created_at, updated_at, is_deleted
			FROM comments
			WHERE post_id = ? AND created_at < ? AND depth = 1 ALLOW FILTERING
		`, pid, cursorTime).WithContext(ctx).PageSize(fetchLimit).Iter()
	}

	var roots []Comment
	var commentID, parentID, userID gocql.UUID
	var content string
	var depth int
	var createdAt, updatedAt time.Time
	var isDeleted bool

	for iter.Scan(&commentID, &parentID, &userID, &content, &depth, &createdAt, &updatedAt, &isDeleted) {
		c := Comment{
			ID:        commentID.String(),
			PostID:    postID,
			UserID:    userID.String(),
			Content:   content,
			Depth:     depth,
			CreatedAt: createdAt,
			IsDeleted: isDeleted,
		}
		if !updatedAt.IsZero() {
			c.UpdatedAt = &updatedAt
		}
		roots = append(roots, c)

		if len(roots) >= limit+1 {
			break
		}
	}

	if err := iter.Close(); err != nil {
		return nil, "", false, fmt.Errorf("failed to get paginated comments: %w", err)
	}

	hasMore := len(roots) > limit
	if hasMore {
		roots = roots[:limit]
	}

	var nextCursor string
	if hasMore && len(roots) > 0 {
		nextCursor = EncodeCursor(roots[len(roots)-1].CreatedAt)
	}

	// Fetch replies for each root
	for i := range roots {
		replies, _, _, _ := r.GetRepliesForComment(ctx, roots[i].ID, 3, "")
		roots[i].Replies = replies
	}

	return roots, nextCursor, hasMore, nil
}

// GetRepliesForComment retrieves replies for a comment
func (r *CommentRepository) GetRepliesForComment(ctx context.Context, parentIDStr string, limit int, cursor string) ([]Comment, string, bool, error) {
	parentComment, err := r.GetCommentByID(ctx, parentIDStr)
	if err != nil {
		return nil, "", false, err
	}

	pid, _ := gocql.ParseUUID(parentComment.PostID)
	parentID, _ := gocql.ParseUUID(parentIDStr)

	if limit <= 0 || limit > 100 {
		limit = 10
	}

	cursorTime, err := DecodeCursor(cursor)
	if err != nil {
		return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
	}

	var iter *gocql.Iter

	if cursorTime.IsZero() {
		iter = r.session.Query(`
			SELECT comment_id, user_id, content, depth, created_at, updated_at, is_deleted
			FROM comments
			WHERE post_id = ? AND parent_id = ? ALLOW FILTERING
		`, pid, parentID).WithContext(ctx).PageSize(limit + 1).Iter()
	} else {
		iter = r.session.Query(`
			SELECT comment_id, user_id, content, depth, created_at, updated_at, is_deleted
			FROM comments
			WHERE post_id = ? AND parent_id = ? AND created_at < ? ALLOW FILTERING
		`, pid, parentID, cursorTime).WithContext(ctx).PageSize(limit + 1).Iter()
	}

	var replies []Comment
	var commentID, userID gocql.UUID
	var content string
	var depth int
	var createdAt, updatedAt time.Time
	var isDeleted bool

	for iter.Scan(&commentID, &userID, &content, &depth, &createdAt, &updatedAt, &isDeleted) {
		c := Comment{
			ID:        commentID.String(),
			PostID:    parentComment.PostID,
			ParentID:  parentIDStr,
			UserID:    userID.String(),
			Content:   content,
			Depth:     depth,
			CreatedAt: createdAt,
			IsDeleted: isDeleted,
		}
		if !updatedAt.IsZero() {
			c.UpdatedAt = &updatedAt
		}
		replies = append(replies, c)

		if len(replies) >= limit+1 {
			break
		}
	}

	if err := iter.Close(); err != nil {
		return nil, "", false, fmt.Errorf("failed to get replies: %w", err)
	}

	// Sort replies ASCending order (oldest first)
	// Actually we need to check if we should reverse. The query returns DESC.
	for i, j := 0, len(replies)-1; i < j; i, j = i+1, j-1 {
		replies[i], replies[j] = replies[j], replies[i]
	}

	hasMore := len(replies) > limit
	if hasMore {
		// since we reversed it, the element to chop is at index 0
		replies = replies[1:]
	}

	var nextCursor string
	if hasMore && len(replies) > 0 {
		// We reversed it, the oldest is at index 0.
		nextCursor = EncodeCursor(replies[0].CreatedAt)
	}

	return replies, nextCursor, hasMore, nil
}

// DeleteComment soft-deletes a comment by its ID
func (r *CommentRepository) DeleteComment(ctx context.Context, commentID, userID string) error {
	comment, err := r.GetCommentByID(ctx, commentID)
	if err != nil {
		return err
	}

	// Only allow owner to delete
	if comment.UserID != userID {
		return fmt.Errorf("unauthorized: can only delete your own comments")
	}

	cid, _ := gocql.ParseUUID(commentID)
	pid, _ := gocql.ParseUUID(comment.PostID)

	batch := r.session.NewBatch(gocql.LoggedBatch)
	batch.WithContext(ctx)

	// Soft Delete from comments_by_id
	batch.Query(`
		UPDATE comments_by_id SET content = '[deleted]', is_deleted = true WHERE comment_id = ?
	`, cid)

	// Soft Delete from comments
	batch.Query(`
		UPDATE comments SET content = '[deleted]', is_deleted = true WHERE post_id = ? AND created_at = ? AND comment_id = ?
	`, pid, comment.CreatedAt, cid)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return fmt.Errorf("ERROR: failed to soft-delete comment: %w", err)
	}

	return nil
}

// GetCommentCount returns the comment count for a post
func (r *CommentRepository) GetCommentCount(ctx context.Context, postID string) (int64, error) {
	if r.commentCounter != nil {
		count, err := r.commentCounter.GetCommentCount(ctx, postID)
		if err == nil && count > 0 {
			return count, nil
		}
	}

	return r.getCommentCountFromCassandra(ctx, postID)
}

func (r *CommentRepository) getCommentCountFromCassandra(ctx context.Context, postID string) (int64, error) {
	pid, err := gocql.ParseUUID(postID)
	if err != nil {
		return 0, fmt.Errorf("invalid post_id: %w", err)
	}

	var count int64
	err = r.session.Query(`
		SELECT count FROM comment_counts WHERE post_id = ?
	`, pid).WithContext(ctx).Scan(&count)
	if err != nil {
		if err == gocql.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}

	return count, nil
}

// GetCommentCountsForPosts returns comment counts for multiple post IDs.
func (r *CommentRepository) GetCommentCountsForPosts(ctx context.Context, postIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(postIDs))
	if len(postIDs) == 0 {
		return counts, nil
	}

	if r.commentCounter != nil {
		redisCounts, err := r.commentCounter.GetCommentCountsBatch(ctx, postIDs)
		if err == nil {
			for postID, count := range redisCounts {
				counts[postID] = count
			}
		}
	}

	// Backfill missing/zero entries from Cassandra to keep API response accurate.
	for _, postID := range postIDs {
		if count, exists := counts[postID]; exists && count > 0 {
			continue
		}

		cassandraCount, err := r.getCommentCountFromCassandra(ctx, postID)
		if err != nil {
			// Keep best-effort behavior for feed/list enrichment.
			if _, exists := counts[postID]; !exists {
				counts[postID] = 0
			}
			continue
		}
		counts[postID] = cassandraCount
	}

	return counts, nil
}
