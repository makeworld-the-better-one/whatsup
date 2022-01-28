package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/makeworld-the-better-one/whatsup/db"
	"github.com/makeworld-the-better-one/whatsup/model"
)

// Status API
// https://github.com/makeworld-the-better-one/fmrl/blob/main/spec.md#status-api

// statusQueryUser encodes to the dictionary for each user in the batch query arry
type statusQueryUser struct {
	Username string        `json:"username"`
	Code     int           `json:"code"`
	Msg      string        `json:"msg,omitempty"`
	Data     *model.Status `json:"data,omitempty"`
}

func statusQuery(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/.well-known/fmrl/users" {
		// Subpaths not allowed
		writeStatusCodePage(w, http.StatusNotFound)
		return
	}

	values := r.URL.Query()

	usernames, ok := values["user"]
	if !ok {
		// No usernames specified
		writeStatusCodePage(w, http.StatusBadRequest)
		return
	}

	users := make([]*statusQueryUser, len(usernames))
	var newest time.Time // Latest status update, for Last-Modified

	// If-Modified-Since
	// Will be the zero value if the header isn't there,
	// which is fine since it would be older than any status update anyway
	var ifModTime time.Time
	if ifm, ok := r.Header["If-Modified-Since"]; ok {
		// Parse time from header
		// Ignore error since the zero value is fine as mentioned above
		ifModTime, _ = http.ParseTime(ifm[0])
	}

	for i, username := range usernames {
		user := &statusQueryUser{Username: username}

		if _, ok := config.Conf.Users[username]; !ok {
			// Username doesn't exist
			user.Code = http.StatusNotFound
			user.Msg = http.StatusText(http.StatusNotFound)
		} else {
			// Username exists
			status, err := db.GetUser(username)

			if err != nil {
				// Log unexpected error
				log.Printf("GetUser(%s): %v", username, err)
				user.Code = http.StatusInternalServerError
				user.Msg = http.StatusText(http.StatusInternalServerError)
			} else if status.UpdatedAt.Before(ifModTime) {
				// Status is already known
				user.Code = 304
			} else {
				user.Code = 200
				user.Data = status

				if status.UpdatedAt.After(newest) {
					newest = status.UpdatedAt
				}
			}
		}

		users[i] = user
	}

	if newest.IsZero() {
		// All users had errors, newest was never set

		if ifModTime.IsZero() {
			// No If-Modified-Since header
			// Set Last-Modified to Unix epoch
			w.Header().Add("Last-Modified", time.Unix(0, 0).UTC().Format(http.TimeFormat))
		} else {
			// Set to same as If-Modified-Since
			w.Header().Add("Last-Modified", ifModTime.UTC().Format(http.TimeFormat))
		}
	} else {
		// newest is set
		w.Header().Add("Last-Modified", newest.UTC().Format(http.TimeFormat))
	}

	apiJSON, err := json.Marshal(users)
	if err != nil {
		log.Printf("JSON encoding for %s : %v", r.URL.RawQuery, err)
		writeStatusCodePage(w, http.StatusInternalServerError)
		return
	}

	// Setting the Content-Type isn't required by the spec, but is nice for checking
	// out API responses in browsers and stuff
	w.Header().Add("Content-Type", "application/json")

	w.Write(apiJSON)
}

func setStatus(w http.ResponseWriter, r *http.Request) {
	// Limit client body to prevent overuse of server resources by malicious
	// clients. 2 KiB is more than enough for a valid JSON body that sets all
	// valid fields.
	r.Body = http.MaxBytesReader(w, r.Body, 2048)

	username := r.URL.Path[len("/.well-known/fmrl/user/"):]

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

	var status model.Status

	if err := json.Unmarshal(clientJSON, &status); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Sent JSON is invalid: %v", err)
		return
	}

	if status.Avatar != nil {
		// Avatar map was set in the status
		// This is not allowed by the spec
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Avatar field should not be set, client not coded correctly")
		return
	}

	if status.IsEmpty() {
		// Nothing was set
		// This is valid but has no effect, and SHOULD NOT
		// update the modified time.
		// So just stop processing, returning 200
		return
	}

	if err := status.Validate(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%v", err)
		return
	}

	err = db.SetUser(username, &status)
	if err != nil {
		log.Printf("SetUser %s: %v", username, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving new status, not your fault.\nContact your server administrator or try again later.")
		return
	}

	// Success!
}

func setAvatar(w http.ResponseWriter, r *http.Request) {
	// Limit to 4 MiB and one extra byte
	// Spec says 4 MiB. By allowing one extra byte it can be determined whether the
	// avatar is too large or just at the limit.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAvatarSize+1)

	username := r.URL.Path[len("/.well-known/fmrl/user/") : len(r.URL.Path)-len("/avatar")]

	if _, ok := config.Conf.Users[username]; !ok {
		// User doesn't exist
		writeStatusCodePage(w, http.StatusNotFound)
		return
	}

	if !checkAuth(username, w, r) {
		return
	}

	if r.Method == "DELETE" {
		err := db.RemoveAvatar(username)
		if err != nil {
			// Logging is done within db.RemoveAvatar, not needed here
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error deleting avatar, not your fault.\nContact your server administrator or try again later.")
			return
		}
		// Success
		return
	}

	imgdata, err := io.ReadAll(r.Body)
	if len(imgdata) == MaxAvatarSize+1 {
		// Request body size is larger than allowed
		// Was limited by http.MaxBytesReader
		writeStatusCodePage(w, http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		log.Printf("setAvatar: reading request body for %s: %v", username, err)
		writeStatusCodePage(w, http.StatusInternalServerError)
		return
	}
	if len(imgdata) == 0 {
		// No request body
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "No avatar image data provided")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(imgdata))
	if errors.Is(err, image.ErrFormat) {
		// Unsupported file type
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Image must be JPEG or PNG only")
		return
	}
	if err != nil {
		// Failed to decode image, assume client error
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Failed to decode image: %v", err)
		return
	}

	if r := img.Bounds(); r.Dx() != r.Dy() {
		// Image is not square
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Image is not square")
		return
	}

	err = db.SetAvatar(username, imgdata)
	if err != nil {
		// Logging is done within db.SetAvatar, not needed here
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving new avatar, not your fault.\nContact your server administrator or try again later.")
		return
	}

	// Success!
}
