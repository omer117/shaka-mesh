package profile

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AvatarURLPrefix is where the gateway serves avatars.
const AvatarURLPrefix = "/media/avatars/"

type Service struct {
	repo    Repository
	blobs   BlobStore
	beaches BeachChecker
}

func NewService(repo Repository, blobs BlobStore, beaches BeachChecker) *Service {
	return &Service{repo: repo, blobs: blobs, beaches: beaches}
}

func (s *Service) Get(ctx context.Context, userID string) (*Profile, error) {
	return s.repo.Get(ctx, userID)
}

// Update applies a partial update to the profile of userID. The caller passes
// the identity from the verified JWT; there is no way to name another user.
func (s *Service) Update(ctx context.Context, userID string, in UpdateInput) (*Profile, error) {
	current, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if utf8.RuneCountInString(name) > MaxDisplayNameRunes {
			return nil, ValidationError{fmt.Sprintf("display_name must be at most %d characters", MaxDisplayNameRunes)}
		}
		if strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return nil, ValidationError{"display_name must not contain control characters"}
		}
		current.DisplayName = name
	}
	if in.Bio != nil {
		bio := strings.TrimSpace(*in.Bio)
		if utf8.RuneCountInString(bio) > MaxBioRunes {
			return nil, ValidationError{fmt.Sprintf("bio must be at most %d characters", MaxBioRunes)}
		}
		current.Bio = bio
	}
	if in.HomeBeachID != nil {
		id := strings.TrimSpace(*in.HomeBeachID)
		if len(id) > MaxHomeBeachIDBytes {
			return nil, ValidationError{"home_beach_id is too long"}
		}
		if id != "" {
			ok, err := s.beaches.Exists(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("verify home beach: %w", err)
			}
			if !ok {
				return nil, ErrBeachNotFound
			}
		}
		current.HomeBeachID = id
	}
	if err := s.repo.Update(ctx, userID, current); err != nil {
		return nil, err
	}
	return current, nil
}

// SetAvatar validates, re-encodes and stores an uploaded image for userID.
func (s *Service) SetAvatar(ctx context.Context, userID string, data []byte) (*Profile, error) {
	processed, err := ProcessAvatar(data)
	if err != nil {
		return nil, err
	}
	key, err := NewAvatarKey()
	if err != nil {
		return nil, err
	}
	if err := s.blobs.Put(ctx, key, processed); err != nil {
		return nil, err
	}
	previous, err := s.repo.SetAvatarURL(ctx, userID, AvatarURLPrefix+key)
	if err != nil {
		if delErr := s.blobs.Delete(ctx, key); delErr != nil {
			log.Printf("clean up orphan avatar %s: %v", key, delErr)
		}
		return nil, err
	}
	if old, ok := strings.CutPrefix(previous, AvatarURLPrefix); ok && old != key {
		if err := s.blobs.Delete(ctx, old); err != nil {
			log.Printf("delete previous avatar %s: %v", old, err)
		}
	}
	return s.repo.Get(ctx, userID)
}
