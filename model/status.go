package model

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/makeworld-the-better-one/go-isemoji"
)

// AvatarMap holds the avatar data for a status.
// When converted to JSON, it includes the avatar number as a query string.
type AvatarMap struct {
	Paths map[string]string
	Num   *int
}

func (av *AvatarMap) MarshalJSON() ([]byte, error) {
	if av.Paths == nil || len(av.Paths) == 0 || av.Paths["original"] == "" {
		// Leave field as null if there's no data rather than have an empty "original" key
		return []byte("null"), nil
	}

	m := make(map[string]string, len(av.Paths))
	for k, v := range av.Paths {
		m[k] = v + "?" + strconv.Itoa(*av.Num)
	}
	return json.Marshal(&m)
}

func (av *AvatarMap) UnmarshalJSON(data []byte) error {
	// Don't bother parsing out the number field because unmarshaling is never
	// supposed to happen anyway.
	// This just exists to prevent any errors if the client incorrectly
	// sends the status with the "avatar" field populated, causing unmarshaling
	// to occur.

	return json.Unmarshal(data, &av.Paths)
}

// Status data for each user.
// Mostly the same fields as JSON.
//
// UpdatedAt to keep track of when it was updated, for the Last-Modified header.
//
// AvatarNum is added as query string to avatar path to keep it unique for
// avatar updates.
//
// Pointers are used for basic types so that an empty value (0 or "") can
// be differentiated from an unset value (nil)
type Status struct {
	Avatar    *AvatarMap `json:"avatar"`
	Name      *string    `json:"name"`
	Status    *string    `json:"status"`
	Emoji     *string    `json:"emoji"`
	Media     *string    `json:"media"`
	MediaType *int       `json:"media_type"`
	URI       *string    `json:"uri"`
	UpdatedAt time.Time  `json:"-"`
}

// IsEmpty returns true if all fields of the status are unset.
// UpdatedAt is ignored.
func (s *Status) IsEmpty() bool {
	if s.Avatar == nil &&
		s.Name == nil &&
		s.Status == nil &&
		s.Emoji == nil &&
		s.Media == nil &&
		s.MediaType == nil &&
		s.URI == nil {

		return true
	}
	return false
}

// Valid returns a error indicating how the status is invalid according
// to the spec. UpdatedAt is ignored.
func (s *Status) Validate() error {
	if s.Avatar != nil && s.Avatar.Paths["original"] == "" {
		return errors.New(`avatar map exists but "original" isn't set`)
	}
	if s.Name != nil && !validString(*s.Name, 40) {
		return errors.New("name is longer than 40 code points or contains control characters")
	}
	if s.Status != nil && !validString(*s.Status, 100) {
		return errors.New("status is longer than 100 code points or contains control characters")
	}
	if s.Emoji != nil && !isemoji.IsEmoji(*s.Emoji) {
		return errors.New("emoji field is not a single fully-qualified emoji")
	}
	if s.Media != nil && !validString(*s.Media, 100) {
		return errors.New("media is longer than 100 code points or contains control characters")
	}
	if s.MediaType != nil && (*s.MediaType < 0 || *s.MediaType > 5) {
		return errors.New("media_type value is undefined")
	}
	if s.URI != nil {
		if len(*s.URI) > 512 || strings.ContainsAny(*s.URI, " \t\r\n") || !strings.ContainsRune(*s.URI, ':') {
			return errors.New("uri field is longer than 512 bytes, contains whitespace, or doesn't have a colon")
		}
		if _, err := url.Parse(*s.URI); err != nil {
			return errors.New("uri field is not a valid URI")
		}
	}
	return nil
}

// validString returns false if the provided string has any characters
// defined as control characters by Unicode, or if it has more code points
// than the provided max.s
func validString(s string, max int) bool {
	// Control characters:
	// https://github.com/makeworld-the-better-one/fmrl/blob/main/spec.md#string-cleaning

	for i, r := range s {
		if i == max {
			return false
		}
		if r <= 0x1F || r == 0x7F || (r >= 0x80 && r <= 0x9F) {
			return false
		}
	}
	return true
}
