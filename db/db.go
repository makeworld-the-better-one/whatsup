// db handles all work with the database and any stored data.
//
// Unless otherwise specified, no functions do any logging.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/makeworld-the-better-one/whatsup/model"
	_ "modernc.org/sqlite"
)

var db *sql.DB

// avatarMutexes is used to protect the creation of avatars
var avatarMutexes = make(map[string]*sync.Mutex)

var ErrNotFound = errors.New("object not found in database")

func Init() error {
	var err error
	db, err = sql.Open("sqlite", filepath.Join(config.Conf.Data.Dir, "data.db"))
	if err != nil {
		return err
	}

	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS statuses
	(
		username TEXT PRIMARY KEY,
		updated_at DATETIME NOT NULL,
		avatar TEXT NOT NULL,
		avatar_num INT NOT NULL,
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		emoji TEXT NOT NULL,
		media TEXT NOT NULL,
		media_type INT NOT NULL,
		uri TEXT NOT NULL
	)
	`)
	if err != nil {
		return err
	}
	// Table for following API
	// "usernames" column is JSON array of global usernames
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS following
	(
		username TEXT PRIMARY KEY,
		updated_at DATETIME NOT NULL,
		usernames BLOB NOT NULL
	)
	`)
	if err != nil {
		return err
	}

	// Create users in config if they don't exist
	for username := range config.Conf.Users {
		if err := createUser(username); err != nil {
			return err
		}
	}

	return nil
}

func Close() error {
	return db.Close()
}

func userExists(username string) (bool, error) {
	row := db.QueryRow(`SELECT username FROM statuses WHERE username=?`, username)
	var tmp string
	err := row.Scan(&tmp)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

// createUser creates a new user with empty data, only if the user doesn't already exist.
func createUser(username string) error {
	avatarMutexes[username] = &sync.Mutex{}

	exists, err := userExists(username)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = db.Exec(`
	INSERT INTO statuses
	(username, updated_at, avatar, avatar_num, name, status, emoji, media, media_type, uri)
	VALUES (?,?,?,?,?,?,?,?,?,?)
	`, username, time.Now(), "", 0, "", "", "", "", 0, "")

	if err != nil {
		return err
	}

	_, err = db.Exec(`
	INSERT INTO following
	(username, updated_at, usernames)
	VALUES (?,?,?)
	`, username, time.Now(), `[]`)

	return err
}

// GetUser returns the user model for the given username.
// Returns ErrNotFound if the user doesn't exist.
func GetUser(username string) (*model.Status, error) {
	row := db.QueryRow(`
	SELECT updated_at, avatar, avatar_num, name, status, emoji, media, media_type, uri
	FROM statuses
	WHERE username=?
	`, username)

	var status = model.Status{Avatar: &model.AvatarMap{}}
	var avatarOriginal string

	err := row.Scan(&status.UpdatedAt, &avatarOriginal, &status.Avatar.Num, &status.Name, &status.Status,
		&status.Emoji, &status.Media, &status.MediaType, &status.URI)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	status.Avatar.Paths = make(map[string]string)
	status.Avatar.Paths["original"] = avatarOriginal

	return &status, nil
}

// SetUser sets the fields for a user that already exists.
// Fields with nil pointers means the existing data will remain unchanged.
//
// UpdatedAt is always ignored and always set here.
func SetUser(username string, data *model.Status) error {
	// Column names and values to be set
	cols := make([]string, 0)
	args := make([]interface{}, 0)

	if data.Avatar != nil {
		cols = append(cols, "avatar")
		args = append(args, data.Avatar.Paths["original"])

		if data.Avatar.Num != nil {
			cols = append(cols, "avatar_num")
			args = append(args, *data.Avatar.Num)
		}
	}
	if data.Name != nil {
		cols = append(cols, "name")
		args = append(args, *data.Name)
	}
	if data.Status != nil {
		cols = append(cols, "status")
		args = append(args, *data.Status)
	}
	if data.Emoji != nil {
		cols = append(cols, "emoji")
		args = append(args, *data.Emoji)
	}
	if data.Media != nil {
		cols = append(cols, "media")
		args = append(args, *data.Media)
	}
	if data.MediaType != nil {
		cols = append(cols, "media_type")
		args = append(args, *data.MediaType)
	}
	if data.URI != nil {
		cols = append(cols, "uri")
		args = append(args, *data.URI)
	}

	cols = append(cols, "updated_at")
	args = append(args, time.Now(), username) // Add username for "WHERE" in statement

	// Construct SQL statement
	stmt := `UPDATE statuses SET `
	stmt += strings.Join(cols, `=?, `) + `=? WHERE username=?`

	_, err := db.Exec(stmt, args...)
	return err
}

// SetAvatar sets the avatar image for a user. The user must exist already.
//
// Due to the diverse set of possible issues this function could encounter,
// it does logging of errors internally. If it returns an error, it doesn't
// need to be logged, because this function will have already logged it.
func SetAvatar(username string, img []byte) error {
	avatarMutexes[username].Lock()
	defer avatarMutexes[username].Unlock()

	dir := filepath.Join(config.Conf.Data.Dir, "avatars", username)
	err := os.Mkdir(dir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		log.Printf("SetAvatar: creating %s : %v", dir, err)
		return err
	}

	imgpath := filepath.Join(dir, "original")
	f, err := os.OpenFile(imgpath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {

		log.Printf("SetAvatar: creating %s : %v", imgpath, err)
		return err
	}
	defer f.Close()

	_, err = f.Write(img)
	if err != nil {
		log.Printf("SetAvatar: writing to file for %s: %v", username, err)
		return err
	}

	// Set user avatar field in case it was blank before, and increment avatar num

	m := make(map[string]string, 1)
	m["original"] = "/fmrl/avatars/" + username

	row := db.QueryRow(`
	SELECT avatar_num
	FROM statuses
	WHERE username=?
	`, username)

	var prevAvatarNum int
	err = row.Scan(&prevAvatarNum)
	if err != nil {
		log.Printf("SetAvatar: get avatar_num for %s: %v", username, err)
		return err
	}

	prevAvatarNum++

	avatarPaths := make(map[string]string, 1)
	avatarPaths["original"] = "/.well-known/fmrl/avatars/" + username + "/original"

	err = SetUser(username,
		&model.Status{
			Avatar: &model.AvatarMap{Paths: avatarPaths, Num: &prevAvatarNum},
		},
	)
	if err != nil {
		log.Printf("SetAvatar: SetUser for %s: %v", username, err)
		return err
	}
	return nil
}

// RemoveAvatar removes the avatar image for a user. The user must exist already.
//
// Due to the diverse set of possible issues this function could encounter,
// it does logging of errors internally. If it returns an error, it doesn't
// need to be logged, because this function will have already logged it.
func RemoveAvatar(username string) error {
	avatarMutexes[username].Lock()
	defer avatarMutexes[username].Unlock()

	imgpath := filepath.Join(config.Conf.Data.Dir, "avatars", username, "original")
	err := os.Remove(imgpath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Unexpected error
		log.Printf("RemoveAvatar: removing %s : %v", imgpath, err)
		return err
	}

	// Even if image does not exist on the filesystem, remove avatar entry in DB
	// just in case
	//
	// Leave avatar_num as is to prevent repetition of avatar string if new avatar
	// is set later

	status := model.Status{
		Avatar: &model.AvatarMap{
			Paths: make(map[string]string), // "original" key will be empty
		},
	}

	err = SetUser(username, &status)
	if err != nil {
		log.Printf("RemoveAvatar: SetUser for %s: %v", username, err)
		return err
	}

	return nil
}

// GetFollowing returns the following list for the given username.
// Returns ErrNotFound if the user doesn't exist.
func GetFollowing(username string) (*model.Following, error) {
	row := db.QueryRow(`
	SELECT updated_at, usernames
	FROM following
	WHERE username=?
	`, username)

	fw := model.Following{Usernames: model.NewFollowingUsernames()}
	var jsonArray []byte

	err := row.Scan(&fw.UpdatedAt, &jsonArray)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonArray, &fw.Usernames); err != nil {
		return nil, err
	}
	return &fw, nil
}

// GetFollowingRaw is like GetFollowing but it returns the JSON-encoded usernames array
// from the database, unparsed.
func GetFollowingRaw(username string) ([]byte, time.Time, error) {
	row := db.QueryRow(`
	SELECT updated_at, usernames
	FROM following
	WHERE username=?
	`, username)

	var usernames []byte
	var updatedAt time.Time

	err := row.Scan(&updatedAt, &usernames)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	return usernames, updatedAt, nil
}

// SetFollowing sets the following usernames for a user that already exists.
//
// If JSONArray is nil it is ignored.
//
// UpdatedAt is always ignored and always set here.
func SetFollowing(username string, data *model.Following) error {
	if data.Usernames == nil {
		return nil
	}

	jsonArray, err := json.Marshal(&data.Usernames)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
	UPDATE following
	SET updated_at=?, usernames=?
	WHERE username=?
	`, time.Now(), jsonArray, username)
	return err
}
