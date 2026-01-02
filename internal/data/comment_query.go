package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

type CommentRepository struct {
	session *gocql.Session
}

func NewCommentRepository(session *gocql.Session) *CommentRepository {
	return &CommentRepository{session: session}
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
		INSERT INTO comments (post_id, comment_id, parent_id, user_id, content, depth, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, postID, commentID, parentID, userID, req.Content, depth, req.IPAddress, req.UserAgent, now)

	// Insert into comments_by_id for direct lookups
	batch.Query(`
		INSERT INTO comments_by_id (comment_id, post_id, parent_id, user_id, content, depth, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, commentID, postID, parentID, userID, req.Content, depth, req.IPAddress, req.UserAgent, now)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return nil, fmt.Errorf("ERROR: failed to create comment: %w", err)
	}

	// Increment comment count for post
	err = r.session.Query(`
		UPDATE comment_counts SET count = count + 1
		WHERE post_id = ?
	`, postID).WithContext(ctx).Exec()
	if err != nil {
		fmt.Printf("Warning: failed to update comment count: %v\n", err)
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
	}, nil
}

// GetCommentByID retrieves a comment by its ID
func (r *CommentRepository) GetCommentByID(ctx context.Context, id string) (*Comment, error) {
	commentID, err := gocql.ParseUUID(id)
	if err != nil {
		return nil, fmt.Errorf("invalid comment_id: %w", err)
	}

	var comment Comment
	var postID, parentID, userID gocql.UUID

	err = r.session.Query(`
		SELECT comment_id, post_id, parent_id, user_id, content, depth, created_at
		FROM comments_by_id
		WHERE comment_id = ?
	`, commentID).WithContext(ctx).Scan(
		&commentID, &postID, &parentID, &userID, &comment.Content, &comment.Depth, &comment.CreatedAt,
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
	if parentID.String() != "00000000-0000-0000-0000-000000000000" {
		comment.ParentID = parentID.String()
	}

	return &comment, nil
}

// GetCommentsForPost retrieves all comments for a post with nested structure
func (r *CommentRepository) GetCommentsForPost(ctx context.Context, postID string, limit int) ([]Comment, error) {
	pid, err := gocql.ParseUUID(postID)
	if err != nil {
		return nil, fmt.Errorf("invalid post_id: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.session.Query(`
		SELECT comment_id, parent_id, user_id, content, depth, created_at
		FROM comments
		WHERE post_id = ?
		LIMIT ?
	`, pid, limit).WithContext(ctx).Iter()

	var allComments []Comment
	var commentID, parentID, userID gocql.UUID
	var content string
	var depth int
	var createdAt time.Time

	for iter.Scan(&commentID, &parentID, &userID, &content, &depth, &createdAt) {
		c := Comment{
			ID:        commentID.String(),
			PostID:    postID,
			UserID:    userID.String(),
			Content:   content,
			Depth:     depth,
			CreatedAt: createdAt,
		}
		if parentID.String() != "00000000-0000-0000-0000-000000000000" {
			c.ParentID = parentID.String()
		}
		allComments = append(allComments, c)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	// Build nested structure
	return buildNestedComments(allComments), nil
}

// buildNestedComments organizes flat comments into nested structure
func buildNestedComments(comments []Comment) []Comment {
	commentMap := make(map[string]*Comment)
	var roots []Comment

	// First pass: create map
	for i := range comments {
		commentMap[comments[i].ID] = &comments[i]
	}

	// Second pass: build tree
	for i := range comments {
		if comments[i].ParentID == "" {
			roots = append(roots, comments[i])
		} else {
			if parent, ok := commentMap[comments[i].ParentID]; ok {
				parent.Replies = append(parent.Replies, comments[i])
			}
		}
	}

	return roots
}

// DeleteComment deletes a comment by its ID
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

	// Delete from comments_by_id
	batch.Query(`
		DELETE FROM comments_by_id WHERE comment_id = ?
	`, cid)

	// Delete from comments (need created_at for deletion)
	batch.Query(`
		DELETE FROM comments WHERE post_id = ? AND created_at = ? AND comment_id = ?
	`, pid, comment.CreatedAt, cid)

	err = r.session.ExecuteBatch(batch)
	if err != nil {
		return fmt.Errorf("ERROR: failed to delete comment: %w", err)
	}

	// Decrement comment count
	err = r.session.Query(`
		UPDATE comment_counts SET count = count - 1 WHERE post_id = ?
	`, pid).WithContext(ctx).Exec()
	if err != nil {
		fmt.Printf("Warning failed to decrement comment_counts: %v", err)
	}

	return nil
}

// GetCommentCount returns the comment count for a post
func (r *CommentRepository) GetCommentCount(ctx context.Context, postID string) (int64, error) {
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
