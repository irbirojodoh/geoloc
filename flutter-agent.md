# Flutter Agent Prompt - Geoloc Mobile App

## Objective

Initialize a Flutter project for **Geoloc**, a hyper-local social media app. The backend is already built with Go + Cassandra. Your job is to create the Flutter client targeting **iOS first**.

---

## Backend Context

### Tech Stack (Backend - Already Built)
- **Language**: Go 1.21+ with Gin framework
- **Database**: Apache Cassandra 4.1
- **Auth**: JWT (15-min access token, 7-day refresh token)
- **Geospatial**: Geohashing (5-char prefix = ~5km precision)
- **Base URL**: `http://localhost:8080` (dev)

---

## App Features to Support

1. **Authentication** - Register, login, JWT token management with auto-refresh
2. **Geolocation Feed** - Posts near user's location with configurable radius
3. **Posts** - Create posts with text + media + location, like/unlike
4. **Comments** - Nested comments up to 3 levels deep, with likes
5. **User Profiles** - View/edit profile, avatar upload
6. **Follow System** - Follow/unfollow users, view followers/following
7. **Location Following** - Subscribe to geographic areas by geohash
8. **Notifications** - In-app notifications for social activities
9. **Search** - Search users and posts
10. **Push Notifications** - Device token registration for push

---

## API Endpoints Reference

### Auth (Public)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/register` | Register new user |
| POST | `/auth/login` | Login, receive JWT tokens |
| POST | `/auth/refresh` | Refresh access token |

### Protected Endpoints (Require `Authorization: Bearer <token>`)

**Feed**
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/feed?latitude=&longitude=&radius_km=&limit=&cursor=` | Get nearby posts (paginated) |

### Protected Endpoints (Require `Authorization: Bearer <token>`)

**Profile & Users**
- `PUT /users/me` - Update current user profile
- `GET /users/:id` - Get user by ID
- `GET /users/:id/posts` - Get user's posts

**Follows**
- `POST /users/:id/follow` - Follow user
- `DELETE /users/:id/follow` - Unfollow user
- `GET /users/:id/followers` - Get user's followers
- `GET /users/:id/following` - Get user's following

**Posts**
- `POST /posts` - Create post (with location)
- `GET /posts/:id` - Get single post
- `POST /posts/:id/like` - Like post
- `DELETE /posts/:id/like` - Unlike post
- `POST /posts/:id/comments` - Add comment
- `GET /posts/:id/comments` - Get comments

**Comments**
- `POST /comments/:id/reply` - Reply to comment (max depth: 3)
- `POST /comments/:id/like` - Like comment
- `DELETE /comments/:id/like` - Unlike comment
- `DELETE /comments/:id` - Delete own comment

**Locations**
- `POST /locations/follow` - Follow a geographic area
- `DELETE /locations/:geohash/follow` - Unfollow area
- `GET /locations/following` - Get followed locations

**Notifications**
- `GET /notifications` - Get all notifications
- `PUT /notifications/:id/read` - Mark as read
- `PUT /notifications/read-all` - Mark all as read

**Search**
- `GET /search/users?q=` - Search users
- `GET /search/posts?q=` - Search posts

**Upload**
- `POST /upload/avatar` - Upload avatar (max 5MB, multipart/form-data)
- `POST /upload/post` - Upload post media (max 50MB, multipart/form-data)

**Devices (Push Notifications)**
- `POST /devices` - Register push token
- `DELETE /devices` - Unregister push token

---

## Data Models

### User
```dart
class User {
  final String id;           // UUID
  final String username;
  final String email;
  final String? fullName;
  final String? bio;
  final String? phoneNumber;
  final String? profilePictureUrl;
  final DateTime? lastOnline;
  final DateTime createdAt;
  final DateTime updatedAt;
  final int followersCount;
  final int followingCount;
}
```

### Post
```dart
class Post {
  final String id;           // UUID
  final String userId;       // UUID
  final String? username;    // Author's username
  final String? profilePictureUrl;
  final String content;
  final List<String> mediaUrls;
  final double latitude;
  final double longitude;
  final String geohash;
  final String? locationName; // e.g., "Kukusan"
  final Address? address;     // Full address object
  final DateTime createdAt;
  final double? distanceKm;   // Distance from query location
  final int likeCount;
  final int commentCount;
  final bool isLiked;        // Current user's like status
}

class Address {
  final String? village;
  final String? cityDistrict;
  final String? city;
  final String? state;
  final String? region;
  final String? postcode;
  final String? country;
  final String? countryCode;
}
```

### Comment
```dart
class Comment {
  final String id;           // UUID
  final String postId;       // UUID
  final String? parentId;    // UUID (null for top-level)
  final String userId;       // UUID
  final String content;
  final int depth;           // 1, 2, or 3 (max)
  final DateTime createdAt;
  final int likeCount;
  final bool isLiked;
  final User? author;
  final List<Comment> replies; // Nested replies
}
```

### Notification
```dart
class AppNotification {
  final String id;           // UUID
  final String type;         // 'like', 'comment', 'follow', 'location_post'
  final String actorId;      // UUID - who triggered it
  final String targetType;   // 'post', 'comment', 'user'
  final String targetId;     // UUID
  final String message;
  final bool isRead;
  final DateTime createdAt;
  final User? actor;         // Embedded actor info
}
```

### LocationFollow
```dart
class LocationFollow {
  final String geohashPrefix;
  final String? name;
  final double latitude;
  final double longitude;
  final DateTime createdAt;
}
```

### AuthTokens
```dart
class AuthTokens {
  final String accessToken;
  final String refreshToken;
  final DateTime accessTokenExpiry;  // 15 minutes
  final DateTime refreshTokenExpiry; // 7 days
}
```

---

## Project Structure (Recommended)

```
lib/
├── main.dart
├── app.dart
├── config/
│   ├── app_config.dart          # API base URL, constants
│   ├── routes.dart              # Route definitions
│   └── theme.dart               # App theme
├── core/
│   ├── constants/
│   ├── errors/
│   │   └── failures.dart        # Custom failure classes
│   ├── network/
│   │   ├── api_client.dart      # Dio setup with interceptors
│   │   ├── auth_interceptor.dart # JWT refresh logic
│   │   └── api_endpoints.dart
│   └── utils/
│       ├── location_utils.dart  # Geolocation helpers
│       └── date_utils.dart
├── data/
│   ├── models/                  # Data models (from above)
│   ├── repositories/            # Repository implementations
│   └── datasources/
│       ├── local/               # Local storage (Hive/SQLite)
│       └── remote/              # API data sources
├── domain/
│   ├── entities/
│   ├── repositories/            # Abstract repository interfaces
│   └── usecases/
├── presentation/
│   ├── screens/
│   │   ├── auth/
│   │   │   ├── login_screen.dart
│   │   │   └── register_screen.dart
│   │   ├── feed/
│   │   │   └── feed_screen.dart
│   │   ├── post/
│   │   │   ├── create_post_screen.dart
│   │   │   └── post_detail_screen.dart
│   │   ├── profile/
│   │   │   ├── profile_screen.dart
│   │   │   └── edit_profile_screen.dart
│   │   ├── search/
│   │   │   └── search_screen.dart
│   │   └── notifications/
│   │       └── notifications_screen.dart
│   ├── widgets/
│   │   ├── post_card.dart
│   │   ├── comment_tile.dart
│   │   ├── user_avatar.dart
│   │   └── location_picker.dart
│   └── providers/               # State management (Riverpod)
└── services/
    ├── auth_service.dart
    ├── location_service.dart
    └── push_notification_service.dart
```

---

## Required Packages

```yaml
dependencies:
  flutter:
    sdk: flutter
  
  # State Management
  flutter_riverpod: ^2.4.9
  
  # Networking
  dio: ^5.4.0
  
  # Local Storage
  flutter_secure_storage: ^9.0.0   # For tokens
  hive_flutter: ^1.1.0             # For caching
  
  # Location
  geolocator: ^10.1.0
  geocoding: ^2.1.1
  
  # Media
  image_picker: ^1.0.7
  image_cropper: ^5.0.1
  cached_network_image: ^3.3.1
  video_player: ^2.8.2
  
  # Push Notifications
  firebase_core: ^2.24.2
  firebase_messaging: ^14.7.10
  
  # UI
  flutter_svg: ^2.0.9
  shimmer: ^3.0.0                  # Loading skeletons
  pull_to_refresh: ^2.0.0
  infinite_scroll_pagination: ^4.0.0
  
  # Utils
  intl: ^0.18.1                    # Date formatting
  timeago: ^3.6.1                  # "5 min ago"
  uuid: ^4.3.3
  
  # Navigation
  go_router: ^13.1.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.1
  build_runner: ^2.4.8
  hive_generator: ^2.0.1
```

---

## iOS-Specific Setup Required

### 1. Location Permissions (ios/Runner/Info.plist)
```xml
<key>NSLocationWhenInUseUsageDescription</key>
<string>Geoloc needs your location to show posts near you</string>
<key>NSLocationAlwaysAndWhenInUseUsageDescription</key>
<string>Geoloc needs your location to notify you about nearby posts</string>
<key>NSLocationAlwaysUsageDescription</key>
<string>Geoloc needs your location to notify you about nearby posts</string>
```

### 2. Camera & Photo Library (ios/Runner/Info.plist)
```xml
<key>NSCameraUsageDescription</key>
<string>Geoloc needs camera access to take photos for posts</string>
<key>NSPhotoLibraryUsageDescription</key>
<string>Geoloc needs photo library access to select images for posts</string>
<key>NSMicrophoneUsageDescription</key>
<string>Geoloc needs microphone access to record videos for posts</string>
```

### 3. Push Notifications
- Enable Push Notifications capability in Xcode
- Configure Firebase for iOS
- Add `GoogleService-Info.plist` to ios/Runner/

### 4. Minimum iOS Version
Set minimum deployment target to iOS 13.0 or higher in:
- `ios/Podfile`: `platform :ios, '13.0'`
- Xcode project settings

---

## Key Implementation Notes

### 1. JWT Token Refresh Flow
```dart
// In auth_interceptor.dart
// - Check if access token is expired before each request
// - If expired, call /auth/refresh with refresh token
// - If refresh fails (401), logout user and redirect to login
// - Store tokens in flutter_secure_storage
```

### 2. Location Flow
```dart
// On app start:
// 1. Check location permission
// 2. If denied, show permission rationale screen
// 3. If granted, get current location
// 4. Fetch nearby feed with location

// On feed refresh:
// 1. Get fresh location
// 2. Call /api/v1/feed with lat/lng/radius
```

### 3. Nested Comments Rendering
```dart
// Comments have depth: 1, 2, or 3
// Render with indentation based on depth
// Disable reply button when depth == 3
// Use recursive widget for nested structure
```

### 4. Error Response Format
```dart
// Backend returns:
// { "error": "message", "details": "optional" }
// Create consistent error handling across app
```

### 5. Rate Limiting
```dart
// Backend: 100 requests/minute
// Implement:
// - Debounce search input (300-500ms)
// - Exponential backoff on 429 errors
// - Cache feed results
```

### 6. Feed Screen Implementation (Cursor Pagination)
```dart
// Feed is a PROTECTED endpoint - requires JWT token
// Uses cursor-based pagination for infinite scroll

class FeedProvider extends ChangeNotifier {
  final ApiClient api;
  final LocationService location;
  
  List<Post> posts = [];
  String? nextCursor;
  bool hasMore = true;
  bool isLoading = false;
  
  Future<void> loadInitialFeed() async {
    posts = [];
    nextCursor = null;
    hasMore = true;
    await loadMore();
  }
  
  Future<void> loadMore() async {
    if (!hasMore || isLoading) return;
    isLoading = true;
    notifyListeners();
    
    try {
      final position = await location.getCurrentPosition();
      final response = await api.get(
        '/api/v1/feed',
        queryParameters: {
          'latitude': position.latitude,
          'longitude': position.longitude,
          'radius_km': 10,
          'limit': 20,
          if (nextCursor != null) 'cursor': nextCursor,
        },
      );
      
      final data = response.data;
      final newPosts = (data['data'] as List)
          .map((json) => Post.fromJson(json))
          .toList();
          
      posts.addAll(newPosts);
      nextCursor = data['next_cursor'];
      hasMore = data['has_more'] ?? false;
    } catch (e) {
      // Handle 401 - redirect to login
      // Handle other errors
    }
    
    isLoading = false;
    notifyListeners();
  }
}
```

### 7. Displaying Location in Posts
```dart
// Each post now includes location_name and address object
Widget buildLocationChip(Post post) {
  final location = post.address?.village ?? 
                   post.address?.cityDistrict ??
                   post.locationName ??
                   'Unknown';
  final city = post.address?.city ?? '';
  
  return Row(
    children: [
      Icon(Icons.location_on, size: 14),
      SizedBox(width: 4),
      Text(
        '$location${city.isNotEmpty ? ', $city' : ''}',
        style: TextStyle(fontSize: 12, color: Colors.grey),
      ),
    ],
  );
}

// Example address object from API:
// {
//   "village": "Kukusan",
//   "city_district": "Beji",
//   "city": "Depok",
//   "state": "West Java",
//   "region": "Java",
//   "postcode": "16425",
//   "country": "Indonesia",
//   "country_code": "id"
// }
```

---

## Initial Tasks Checklist

1. [ ] Create new Flutter project with `flutter create --org com.yourcompany geoloc_app`
2. [ ] Set up folder structure as shown above
3. [ ] Configure `pubspec.yaml` with required packages
4. [ ] Set up iOS permissions in `Info.plist`
5. [ ] Create `ApiClient` with Dio and auth interceptor
6. [ ] Implement `AuthService` with login/register/refresh
7. [ ] Implement `LocationService` with permission handling
8. [ ] Create data models (User, Post, Comment, etc.)
9. [ ] Build auth screens (Login, Register)
10. [ ] Build main feed screen with location-based posts
11. [ ] Implement post creation with media upload
12. [ ] Add post detail with comments (nested)
13. [ ] Build profile screen with edit functionality
14. [ ] Implement follow/unfollow system
15. [ ] Add search functionality
16. [ ] Build notifications screen
17. [ ] Set up Firebase for push notifications
18. [ ] Add location following feature
19. [ ] Implement offline caching for feed
20. [ ] Polish UI with animations and loading states

---

## Design Guidelines

- Use a modern, clean design with dark mode support
- Implement smooth animations for likes, comments, and transitions
- Use skeleton loaders (shimmer) for loading states
- Design for iOS first (follow Apple HIG)
- Consider safe areas for notch and home indicator
- Use haptic feedback for important actions

---

## Questions to Consider

1. What is the app name and bundle identifier?
2. What color palette/branding to use?
3. Should we implement biometric auth (Face ID/Touch ID)?
4. What's the default feed radius (5km, 10km)?
5. Should we support offline posting (queue for later)?
6. Any specific analytics or crash reporting tools to integrate?

---

*This prompt contains all context needed to build the Flutter client. Start with authentication and location services as the foundation, then build out the feed and social features.*
