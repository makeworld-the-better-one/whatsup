package model

import (
	"encoding/json"
	"time"
)

// FollowingUsernames is the "set" of global usernames a user follows
type FollowingUsernames map[string]struct{}

func NewFollowingUsernames() FollowingUsernames {
	return make(FollowingUsernames)
}

// Decode and encode to JSON array

func (fu FollowingUsernames) MarshalJSON() ([]byte, error) {
	usernames := make([]string, len(fu))
	i := 0
	for u := range fu {
		usernames[i] = u
		i++
	}
	return json.Marshal(&usernames)
}

func (fu FollowingUsernames) UnmarshalJSON(data []byte) error {
	usernames := make([]string, 0)
	err := json.Unmarshal(data, &usernames)
	if err != nil {
		return err
	}
	for _, u := range usernames {
		fu[u] = struct{}{}
	}
	return nil
}

type Following struct {
	UpdatedAt time.Time
	Usernames FollowingUsernames
}
