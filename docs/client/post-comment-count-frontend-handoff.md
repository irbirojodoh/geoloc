# Frontend handoff — `comment_count` on post payloads

Audience: frontend/mobile agent implementing post UI counters.

## What changed

Backend now includes `comment_count` in all routes that return `Post` objects.

`Post` shape now includes:

```json
{
  "id": "post-uuid",
  "like_count": 42,
  "comment_count": 8,
  "is_liked": false
}
```

## Endpoints affected

- `GET /api/v1/feed` (`data[]`)
- `GET /api/v1/posts/:id` (`post`)
- `GET /api/v1/users/:id/posts` (`data[]`)
- `GET /api/v1/search/posts` (`results[]`, legacy Cassandra search)
- `GET /api/v1/search` (`posts[]`, ES-backed)
- `GET /api/v1/search/nearby` (`posts[]`, ES-backed)
- `POST /api/v1/posts` (`post` object in create response now has `comment_count: 0`)

## Frontend implementation notes

- Use backend `comment_count` directly for badges/counters in feed, detail, profile posts, and search lists.
- Keep `GET /api/v1/posts/:id/comments` `total_count` as source of truth after opening comments screen if needed.
- For optimistic comment creation:
  - increment local `comment_count` immediately;
  - reconcile with server response on refresh/fetch.

## Backward compatibility

- If client code supports older backend versions, handle missing `comment_count` with default `0`.

Example:

```ts
const commentCount = post.comment_count ?? 0;
```

