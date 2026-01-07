# Comments API

Endpoints for the nested comments system. Comments support up to 3 levels of nesting.

## Add Comment

**Endpoint:** `POST /api/v1/posts/:id/comments`

**Request:**
```json
{
  "content": "Great photo!"
}
```

**Response:** `201 Created`
```json
{
  "id": "comment-uuid",
  "post_id": "post-uuid",
  "user_id": "user-uuid",
  "content": "Great photo!",
  "depth": 1,
  "created_at": "2026-01-05T10:30:00Z"
}
```

## Get Comments

**Endpoint:** `GET /api/v1/posts/:id/comments`

Returns nested comment structure.

**Response:** `200 OK`
```json
{
  "count": 5,
  "comments": [
    {
      "id": "comment-1",
      "content": "Great photo!",
      "depth": 1,
      "user": {
        "id": "user-uuid",
        "username": "commenter",
        "profile_picture_url": "..."
      },
      "replies": [
        {
          "id": "comment-2",
          "content": "Thanks!",
          "depth": 2,
          "replies": []
        }
      ]
    }
  ]
}
```

## Reply to Comment

**Endpoint:** `POST /api/v1/comments/:id/reply`

> **Note:** Maximum depth is 3. Cannot reply to depth-3 comments.

**Request:**
```json
{
  "content": "Thanks!"
}
```

**Response:** `201 Created`
```json
{
  "id": "reply-uuid",
  "parent_id": "comment-uuid",
  "content": "Thanks!",
  "depth": 2,
  "created_at": "2026-01-05T10:31:00Z"
}
```

## Like Comment

**Endpoint:** `POST /api/v1/comments/:id/like`

**Response:** `200 OK`
```json
{
  "message": "Comment liked"
}
```

## Unlike Comment

**Endpoint:** `DELETE /api/v1/comments/:id/like`

**Response:** `200 OK`
```json
{
  "message": "Comment unliked"
}
```

## Delete Comment

**Endpoint:** `DELETE /api/v1/comments/:id`

> Only the comment author can delete their comment.

**Response:** `200 OK`
```json
{
  "message": "Comment deleted"
}
```

## Comment Depth

| Depth | Description |
|-------|-------------|
| 1 | Top-level comment on post |
| 2 | Reply to depth-1 comment |
| 3 | Reply to depth-2 comment (max) |

Replies to depth-3 comments are not allowed.
