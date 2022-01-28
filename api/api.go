package api

import (
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/makeworld-the-better-one/whatsup/version"
)

const MaxAvatarSize = 4 * 1024 * 1024 // 4 MiB, from spec

type Server struct {
	serveMux http.ServeMux
}

func NewServer() *Server {
	s := &Server{}

	// All paths, even non-API ones, are under /fmrl/
	// So that reverse-proxying can work under a specific path only

	// API calls
	s.serveMux.HandleFunc("/.well-known/fmrl/user/", userPath)
	s.serveMux.HandleFunc("/.well-known/fmrl/users", CORS(statusQuery))
	// File server of avatar images
	s.serveMux.Handle("/.well-known/fmrl/avatars/",
		http.StripPrefix("/.well-known/fmrl/avatars/",
			http.FileServer(http.Dir(filepath.Join(config.Conf.Data.Dir, "avatars"))),
		),
	)
	// Not in the spec, just a nice way to see what version people are running
	s.serveMux.HandleFunc("/.well-known/fmrl/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, version.VersionInfo)
	})
	return s
}

func userPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "PATCH" && r.Method != "DELETE" && r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/.well-known/fmrl/user/" {
		// No user specified
		writeStatusCodePage(w, http.StatusNotFound)
		return
	}

	// Dispatch to other http.HandlerFunc

	if r.Method == "PATCH" && strings.Count(r.URL.Path, "/") == 4 {
		// Right method and right path
		setStatus(w, r)
		return
	}
	if (r.Method == "PUT" || r.Method == "DELETE") && strings.HasSuffix(r.URL.Path, "/avatar") &&
		len(r.URL.Path) > len("/.well-known/fmrl/user//avatar") {
		// Right method and path, and username exists in path
		setAvatar(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/following") &&
		len(r.URL.Path) > len("/.well-known/fmrl/user//following") {
		// Right path and username exists in path
		if r.Method == "GET" {
			getFollowing(w, r)
		} else if r.Method == "PATCH" {
			setFollowing(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	writeStatusCodePage(w, http.StatusBadRequest)
}
