# API Integration Best Practices

Guidelines for building clients that integrate with the Geoloc API.

## Authentication Flow

```
┌─────────────┐
│   App Start │
└──────┬──────┘
       │
       ▼
┌──────────────────────┐
│ Has stored tokens?   │
└──────────┬───────────┘
     No    │    Yes
       ┌───┴───┐
       │       │
       ▼       ▼
┌──────────┐ ┌─────────────────┐
│  Login   │ │ Validate token  │
│  Screen  │ │ (try API call)  │
└──────────┘ └────────┬────────┘
                  401 │    200
               ┌──────┴──────┐
               │             │
               ▼             ▼
        ┌───────────┐  ┌───────────┐
        │  Refresh  │  │   Home    │
        │   Token   │  │  Screen   │
        └───────────┘  └───────────┘
```

## Token Refresh Strategy

1. **Proactive**: Check expiry before each request
2. **Reactive**: Handle 401 and refresh on demand

Recommended: **Reactive** with automatic retry.

```dart
// On 401 error:
// 1. Call /auth/refresh with refresh_token
// 2. If success, save new access_token, retry original request
// 3. If fail, clear tokens, redirect to login
```

## Pagination Pattern

```dart
class PaginatedList<T> {
  List<T> items = [];
  String? cursor;
  bool hasMore = true;
  bool isLoading = false;
  
  Future<void> loadNext() async {
    if (!hasMore || isLoading) return;
    isLoading = true;
    
    final response = await api.get(endpoint, queryParameters: {
      'limit': 20,
      if (cursor != null) 'cursor': cursor,
    });
    
    items.addAll(parseItems(response.data['data']));
    cursor = response.data['next_cursor'];
    hasMore = response.data['has_more'] ?? false;
    isLoading = false;
  }
  
  void reset() {
    items = [];
    cursor = null;
    hasMore = true;
  }
}
```

## Error Handling

| Status | Action |
|--------|--------|
| 400 | Show validation errors |
| 401 | Refresh token or logout |
| 403 | Show permission denied |
| 404 | Show not found message |
| 429 | Retry with exponential backoff |
| 500 | Show generic error, log details |

```dart
void handleError(DioException e) {
  switch (e.response?.statusCode) {
    case 400:
      showValidationError(e.response?.data['error']);
      break;
    case 401:
      refreshTokenOrLogout();
      break;
    case 429:
      retryWithBackoff(e.requestOptions);
      break;
    default:
      showGenericError();
  }
}
```

## Caching Strategy

| Data | Strategy |
|------|----------|
| Feed posts | Cache 50 most recent, TTL 5 min |
| User profiles | Cache visited, TTL 10 min |
| Current user | Cache until logout |
| Search results | Don't cache |

## Optimistic Updates

For instant UI feedback:

```dart
// Like button example
void likePost(Post post) async {
  // 1. Optimistically update UI
  post.isLiked = true;
  post.likeCount++;
  notifyListeners();
  
  try {
    // 2. Make API call
    await api.post('/api/v1/posts/${post.id}/like');
  } catch (e) {
    // 3. Revert on failure
    post.isLiked = false;
    post.likeCount--;
    notifyListeners();
    showError('Failed to like post');
  }
}
```

## Rate Limiting

- API limit: 100 requests/minute
- Implement debouncing for search (300-500ms)
- Batch operations where possible

## Offline Support

1. **Cache feed** in local database
2. **Queue actions** (likes, posts, comments) when offline
3. **Sync queue** when connection restored
4. **Show cached data** with "offline" indicator
