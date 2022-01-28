package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/matthewhartstonge/argon2"
)

// Helper functions and middleware

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.serveMux.ServeHTTP(w, r)
}

// writeStatusCodePage returns the status code to the client, and writes the
// default text describing that status code as the document.
func writeStatusCodePage(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
	fmt.Fprint(w, http.StatusText(code))
}

// CORS adds CORS support to the request, as defined by the spec.
// If the request is not GET or OPTIONS, it returns StatusMethodNotAllowed
// to the client.
func CORS(next http.HandlerFunc) http.HandlerFunc {
	// https://github.com/makeworld-the-better-one/fmrl/blob/main/spec.md#cross-origin-resource-sharing-cors
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			w.Header().Add("Access-Control-Allow-Origin", "*")
			w.Header().Add("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Add("Access-Control-Allow-Headers", "If-Modified-Since")
			w.Header().Add("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Set CORS header no matter what error occurs later
		w.Header().Add("Access-Control-Allow-Origin", "*")
		next(w, r)
	}
}

// checkAuth authenticates the request and returns an error response
// if needed. If the return value error is false, an error was sent and further
// processing of the request should stop immediately.
func checkAuth(username string, w http.ResponseWriter, r *http.Request) bool {
	authUsername, password, ok := r.BasicAuth()
	if !ok {
		writeStatusCodePage(w, http.StatusUnauthorized)
		return false
	}

	if authUsername != username {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Authentication username doesn't match username in URL")
		return false
	}

	if _, ok := config.Conf.Users[username]; !ok {
		// User doesn't exist
		writeStatusCodePage(w, http.StatusNotFound)
		return false
	}

	ok, err := argon2.VerifyEncoded([]byte(password), []byte(config.Conf.Users[username]))
	if err != nil {
		log.Printf("setStatus: verifying password for %s: %v", username, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error verifying password, not your fault.\nContact your server administrator or try again later.")
		return false
	}
	if !ok {
		// Password isn't correct
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Incorrect password")
		return false
	}

	return true
}

// dedupStringSlice returns the same slice but with no duplicate elements.
// Order is not preserved.
// This is unused right now, but could be used to prevent returning duplicate
// usernames if the client requests it (which is invalid).
// func dedupStringSlice(ss []string) []string {
// 	// http://rosettacode.org/wiki/Remove_duplicate_elements#Go

// 	unique := make(map[string]bool, len(ss))
// 	for _, s := range ss {
// 		unique[s] = true
// 	}
// 	result := make([]string, 0, len(unique))
// 	for s := range unique {
// 		result = append(result, s)
// 	}
// 	return result
// }
