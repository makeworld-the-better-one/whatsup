package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/makeworld-the-better-one/whatsup/db"
)

// Following API
// https://github.com/makeworld-the-better-one/fmrl/blob/main/spec.md#following-api

func getFollowing(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Path[len("/.well-known/fmrl/user/") : len(r.URL.Path)-len("/following")]

	if _, ok := config.Conf.Users[username]; !ok {
		// User doesn't exist
		writeStatusCodePage(w, http.StatusNotFound)
		return
	}

	if !checkAuth(username, w, r) {
		return
	}

	following, updatedAt, err := db.GetFollowingRaw(username)
	if err != nil {
		log.Printf("GetFollowingRaw(%s): %v", username, err)
		writeStatusCodePage(w, http.StatusInternalServerError)
		return
	}

	w.Header().Add("Last-Modified", updatedAt.UTC().Format(http.TimeFormat))

	// If-Modified-Since
	// Will be the zero value if the header isn't there,
	// which is fine since it would be older than any status update anyway
	var ifModTime time.Time
	if ifm, ok := r.Header["If-Modified-Since"]; ok {
		// Parse time from header
		// Ignore error since the zero value is fine as mentioned above
		ifModTime, _ = http.ParseTime(ifm[0])
	}

	if updatedAt.Before(ifModTime) || updatedAt.Truncate(time.Second).Equal(ifModTime) {
		// No new updates
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Setting the Content-Type isn't required by the spec, but is nice for checking
	// out API responses in browsers and stuff
	w.Header().Add("Content-Type", "application/json")

	w.Write(following)
}

type setFollowingJSON struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

var followingUsernameRE = regexp.MustCompile(`^@[a-z0-9_\.]{1,40}@[\w\.]+$`)

func setFollowing(w http.ResponseWriter, r *http.Request) {
	// Limit client JSON to 1 MiB, more than enough
	r.Body = http.MaxBytesReader(w, r.Body, 1*1024*1024)

	username := r.URL.Path[len("/.well-known/fmrl/user/") : len(r.URL.Path)-len("/following")]

	if _, ok := config.Conf.Users[username]; !ok {
		// User doesn't exist
		writeStatusCodePage(w, http.StatusNotFound)
		return
	}

	if !checkAuth(username, w, r) {
		return
	}

	clientJSON, err := io.ReadAll(r.Body)
	if err != nil {
		// Most likely that the client body was too large, don't log
		writeStatusCodePage(w, http.StatusRequestEntityTooLarge)
		return
	}

	var data setFollowingJSON

	// See setStatus for why
	if !json.Valid(clientJSON) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Received JSON is invalid")
		return
	}

	dec := json.NewDecoder(bytes.NewReader(clientJSON))
	dec.DisallowUnknownFields()

	err = dec.Decode(&data)
	if err != nil {
		// JSON has unknown field
		// Or wrong type in right field
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Bad value type or unrecognized field: %v", err)
		return
	}

	if len(data.Add) == 0 && len(data.Remove) == 0 {
		// Request that did nothing
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "No usernames to add or remove")
		return
	}

	fu, err := db.GetFollowing(username)
	if err != nil {
		log.Printf("db.GetFollowing(%s): %v", username, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error retrieving following list, not your fault.\nContact your server administrator or try again later.")
		return
	}

	for _, u := range data.Add {
		if !followingUsernameRE.MatchString(u) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid global username: %s", u)
			return
		}
		fu.Usernames[u] = struct{}{}
	}
	for _, u := range data.Remove {
		if !followingUsernameRE.MatchString(u) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid global username: %s", u)
			return
		}
		delete(fu.Usernames, u)
	}

	err = db.SetFollowing(username, fu)
	if err != nil {
		log.Printf("db.GetFollowing(%s): %v", username, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving new following list, not your fault.\nContact your server administrator or try again later.")
		return
	}
}
