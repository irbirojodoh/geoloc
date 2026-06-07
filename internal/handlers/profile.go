package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"social-geo-go/internal/auth"
	"social-geo-go/internal/data"
	"social-geo-go/internal/search"
	"social-geo-go/internal/storage"
)

// UpdateProfile handles PUT /api/v1/users/me
func UpdateProfile(
	userRepo *data.UserRepository,
	followRepo *data.FollowRepository,
	searchIndexer search.SearchIndexer,
	store storage.MediaStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := auth.GetUserID(c)
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		var req data.UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request body",
			})
			return
		}

		existing, err := userRepo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load profile"})
			return
		}

		avatarValue := existing.ProfilePictureURL
		if req.AvatarKey != "" {
			if err := validateOwnedMediaKey(req.AvatarKey, userID, "avatars"); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			deleteStoredMediaIfOwned(c, store, userID, existing.ProfilePictureURL)
			avatarValue = storage.StoredMediaValue(store, req.AvatarKey)
		} else if req.ProfilePictureURL != "" {
			avatarValue = req.ProfilePictureURL
		}

		coverValue := existing.CoverImageURL
		if req.CoverKey != "" {
			if err := validateOwnedMediaKey(req.CoverKey, userID, "covers"); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if existing.CoverImageURL != "" && existing.CoverImageURL != data.DefaultCoverImageURL {
				deleteStoredMediaIfOwned(c, store, userID, existing.CoverImageURL)
			}
			coverValue = storage.StoredMediaValue(store, req.CoverKey)
		} else if req.CoverImageURL != "" {
			coverValue = req.CoverImageURL
		}

		fullName := req.FullName
		bio := req.Bio
		phoneNumber := req.PhoneNumber

		user, err := userRepo.UpdateUser(
			c.Request.Context(),
			userID,
			fullName,
			bio,
			phoneNumber,
			avatarValue,
			coverValue,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to update profile",
			})
			return
		}

		followerCount := 0
		if followRepo != nil {
			if counts, err := followRepo.GetFollowCounts(c.Request.Context(), userID); err == nil && counts != nil {
				followerCount = int(counts.FollowersCount)
			}
		}
		search.PublishUserIndexedAsync(searchIndexer, search.UserIndexedEventFromUser(user, followerCount))

		ResolveUserMediaURLs(store, user)

		c.JSON(http.StatusOK, gin.H{
			"message": "Profile updated",
			"user":    user,
		})
	}
}

func validateOwnedMediaKey(key, userID, folder string) error {
	if err := storage.ValidateKey(key); err != nil {
		return err
	}
	if !strings.HasPrefix(key, folder+"/") {
		return errInvalidMediaKey
	}
	ownerID, err := storage.KeyOwnerUserID(key)
	if err != nil {
		return err
	}
	if ownerID != userID {
		return errInvalidMediaKey
	}
	return nil
}

var errInvalidMediaKey = &mediaKeyError{"Invalid media key"}

type mediaKeyError struct{ msg string }

func (e *mediaKeyError) Error() string { return e.msg }

func deleteStoredMediaIfOwned(c *gin.Context, store storage.MediaStore, userID, storedValue string) {
	if store == nil || storedValue == "" {
		return
	}
	key := storage.KeyFromStoredValue(storedValue, store.PublicDomain())
	if key == "" || !storage.IsMediaKey(key) {
		return
	}
	ownerID, err := storage.KeyOwnerUserID(key)
	if err != nil || ownerID != userID {
		return
	}
	if err := store.DeleteObject(c.Request.Context(), key); err != nil {
		slog.Warn("failed to delete previous media object", "key", key, "error", err)
	}
}
