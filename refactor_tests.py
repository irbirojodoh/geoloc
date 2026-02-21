import os
import re

auth_symbols = ["HashPassword", "VerifyPassword", "TokenType", "TokenTypeAccess", "TokenTypeRefresh", "ErrInvalidTokenType", "ErrWrongType", "ErrExpiredToken", "Claims", "GenerateTokenPair", "ValidateAccessToken", "ValidateRefreshToken", "AuthRequired", "GetUserID"]
data_symbols = ["NewUserRepository", "CreateUserRequest", "User", "UpdateProfileRequest", "NewPostRepository", "CreatePostRequest", "Post", "NewCommentRepository", "CreateCommentRequest", "Comment", "NewLikeRepository", "NewFollowRepository", "NewNotificationRepository", "Location", "LocationRepository", "NewLocationRepository", "LikeRepository", "FollowRepository", "CommentRepository", "PostRepository", "UserRepository", "NotificationRepository", "ToggleLikeResult", "Notification", "GetFeedRequest"]
handlers_symbols = ["OAuthLogin", "OAuthCallback", "Register", "Login", "Refresh", "GetFeed", "GetCurrentUser", "UpdateProfile", "GetUser", "GetUserByUsername", "GetUserPosts", "FollowUser", "UnfollowUser", "GetFollowers", "GetFollowing", "CreatePost", "GetPost", "CreateComment", "GetComments", "ReplyToComment", "DeleteComment", "LikePost", "UnlikePost", "TogglePostLike", "LikeComment", "UnlikeComment", "ToggleCommentLike", "SearchUsers", "SearchPosts", "GetNotifications", "MarkNotificationAsRead", "FollowLocation", "GetFollowedLocations", "UnfollowLocation", "SetupRouter", "InitRouter"]

def process_file(filepath, pkg_name, imports, symbol_map):
    with open(filepath, 'r') as f:
        content = f.read()

    # Change package
    content = re.sub(r'^package\s+\w+', f'package {pkg_name}', content, flags=re.MULTILINE)

    # Add imports
    import_block = "import (\n"
    for imp in imports:
        import_block += f'\t"{imp}"\n'
    
    # Simple injection if import () exists
    if "import (" in content:
        content = content.replace("import (", import_block, 1)
    else:
        # fallback
        content = content.replace(f'package {pkg_name}', f'package {pkg_name}\n\n{import_block})\n')

    # Replace symbols
    for prefix, symbols in symbol_map.items():
        for sym in symbols:
            # Word boundary to avoid partial matches
            content = re.sub(r'\b' + sym + r'\b', f'{prefix}.{sym}', content)

    with open(filepath, 'w') as f:
        f.write(content)

# Process unit
process_file("tests/unit/auth_test.go", "unit", ["social-geo-go/internal/auth"], {"auth": auth_symbols})
process_file("tests/unit/oauth_test.go", "unit", ["social-geo-go/internal/handlers", "social-geo-go/internal/auth"], {"handlers": handlers_symbols, "auth": auth_symbols})

# Process integration
for f in os.listdir("tests/integration"):
    if f.endswith(".go"):
        process_file("tests/integration/" + f, "integration", ["social-geo-go/internal/data", "social-geo-go/internal/cache"], {"data": data_symbols})

# Process e2e
for f in os.listdir("tests/e2e"):
    if f.endswith(".go"):
        process_file("tests/e2e/" + f, "e2e", ["social-geo-go/internal/handlers", "social-geo-go/internal/data", "social-geo-go/internal/auth"], {"handlers": handlers_symbols, "data": data_symbols, "auth": auth_symbols})

print("Done")
