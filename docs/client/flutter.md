# Flutter Client Guide

Guide for building the Geoloc mobile app with Flutter.

## Overview

The full Flutter client prompt is available at [flutter-agent.md](../../flutter-agent.md).

This guide covers key implementation patterns.

## Project Setup

```bash
flutter create --org com.yourcompany geoloc_app
cd geoloc_app
```

### Required Packages

```yaml
dependencies:
  # State Management
  flutter_riverpod: ^2.4.9
  
  # Networking
  dio: ^5.4.0
  
  # Storage
  flutter_secure_storage: ^9.0.0
  hive_flutter: ^1.1.0
  
  # Location
  geolocator: ^10.1.0
  
  # Media
  image_picker: ^1.0.7
  cached_network_image: ^3.3.1
  
  # UI
  shimmer: ^3.0.0
  infinite_scroll_pagination: ^4.0.0
```

## Authentication

### Token Storage

```dart
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class TokenStorage {
  static const _storage = FlutterSecureStorage();
  
  static Future<void> saveTokens(String access, String refresh) async {
    await _storage.write(key: 'access_token', value: access);
    await _storage.write(key: 'refresh_token', value: refresh);
  }
  
  static Future<String?> getAccessToken() async {
    return await _storage.read(key: 'access_token');
  }
}
```

### Auth Interceptor

```dart
class AuthInterceptor extends Interceptor {
  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) async {
    final token = await TokenStorage.getAccessToken();
    if (token != null) {
      options.headers['Authorization'] = 'Bearer $token';
    }
    handler.next(options);
  }
  
  @override
  void onError(DioException err, ErrorInterceptorHandler handler) async {
    if (err.response?.statusCode == 401) {
      // Try refresh token
      final refreshed = await _refreshToken();
      if (refreshed) {
        // Retry original request
        return handler.resolve(await _retry(err.requestOptions));
      }
    }
    handler.next(err);
  }
}
```

## Feed Implementation

### Data Model

```dart
class Post {
  final String id;
  final String userId;
  final String? username;
  final String? profilePictureUrl;
  final String content;
  final List<String> mediaUrls;
  final String geohash;
  final String? locationName;
  final Address? address;
  final DateTime createdAt;
  final double? distanceKm;
}

class Address {
  final String? village;
  final String? city;
  final String? country;
}
```

### Feed Provider

```dart
class FeedProvider extends ChangeNotifier {
  List<Post> posts = [];
  String? nextCursor;
  bool hasMore = true;
  bool isLoading = false;
  
  Future<void> loadMore(double lat, double lng) async {
    if (!hasMore || isLoading) return;
    isLoading = true;
    notifyListeners();
    
    final response = await api.get('/api/v1/feed', queryParameters: {
      'latitude': lat,
      'longitude': lng,
      'limit': 20,
      if (nextCursor != null) 'cursor': nextCursor,
    });
    
    final data = response.data;
    posts.addAll((data['data'] as List).map((j) => Post.fromJson(j)));
    nextCursor = data['next_cursor'];
    hasMore = data['has_more'] ?? false;
    isLoading = false;
    notifyListeners();
  }
}
```

## Location Handling

```dart
import 'package:geolocator/geolocator.dart';

Future<Position?> getCurrentLocation() async {
  bool serviceEnabled = await Geolocator.isLocationServiceEnabled();
  if (!serviceEnabled) return null;
  
  LocationPermission permission = await Geolocator.checkPermission();
  if (permission == LocationPermission.denied) {
    permission = await Geolocator.requestPermission();
    if (permission == LocationPermission.denied) return null;
  }
  
  return await Geolocator.getCurrentPosition();
}
```

## Display Location

```dart
Widget buildLocationChip(Post post) {
  final location = post.address?.village ?? 
                   post.locationName ?? 
                   'Unknown';
  final city = post.address?.city ?? '';
  
  return Row(
    children: [
      Icon(Icons.location_on, size: 14),
      SizedBox(width: 4),
      Text('$location${city.isNotEmpty ? ', $city' : ''}'),
    ],
  );
}
```

## iOS Setup

Add to `ios/Runner/Info.plist`:

```xml
<key>NSLocationWhenInUseUsageDescription</key>
<string>Geoloc needs your location to show posts near you</string>
<key>NSCameraUsageDescription</key>
<string>Geoloc needs camera access for photos</string>
<key>NSPhotoLibraryUsageDescription</key>
<string>Geoloc needs photo library access for uploads</string>
```

## Best Practices

1. **Debounce search** - Wait 300ms after typing stops
2. **Cache feed** - Store in Hive for offline access
3. **Lazy load images** - Use `cached_network_image`
4. **Error handling** - Handle 401 → refresh → retry flow
5. **Loading states** - Use shimmer skeletons
