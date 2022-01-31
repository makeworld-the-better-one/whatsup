package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/matthewhartstonge/argon2"
	"golang.org/x/term"

	"github.com/makeworld-the-better-one/whatsup/api"
	"github.com/makeworld-the-better-one/whatsup/config"
	"github.com/makeworld-the-better-one/whatsup/db"
	"github.com/makeworld-the-better-one/whatsup/version"
)

var (
	versionFlag bool
	confFlag    string
)

func main() {

	flag.BoolVar(&versionFlag, "version", false, "See version info")
	flag.StringVar(&confFlag, "config", "/etc/whatsup/config.toml", "Set config path")
	flag.Parse()

	if versionFlag {
		fmt.Print(version.VersionInfo)
		return
	}

	if flag.Arg(0) == "hash" {
		os.Exit(passwordHash())
	}

	log.Println("started")

	if _, err := toml.DecodeFile(confFlag, &config.Conf); err != nil {
		log.Fatal(err)
	}

	err := os.Mkdir(config.Conf.Data.Dir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		log.Fatal(err)
	}

	err = os.Mkdir(filepath.Join(config.Conf.Data.Dir, "avatars"), 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		log.Fatal(err)
	}

	for username := range config.Conf.Users {
		if !validUsername(username) {
			log.Fatal("Username uses invalid characters: " + username)
		}
	}

	if err := db.Init(); err != nil {
		log.Fatal(err)
	}

	apiHandler := api.NewServer()

	s := &http.Server{
		Addr:         net.JoinHostPort(config.Conf.Server.Host, strconv.Itoa(int(config.Conf.Server.Port))),
		Handler:      apiHandler,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
	}
	errc := make(chan error, 1)

	if config.Conf.Server.Cert != "" && config.Conf.Server.Key != "" {
		go func() {
			errc <- s.ListenAndServeTLS(config.Conf.Server.Cert, config.Conf.Server.Key)
		}()
		log.Printf("Listening on https://%s", s.Addr)
	} else {
		go func() {
			errc <- s.ListenAndServe()
		}()
		log.Printf("Listening on http://%s", s.Addr)
	}

	// Wait for server error or process signals (like Ctrl-C)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errc:
		log.Printf("failed to serve: %v", err)
	case sig := <-sigs:
		log.Printf("terminating: %v", sig)
	}

	// Gracefully shut down HTTP server with 5 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Printf("shutting down HTTP server with timeout: %v", err)
	}

	if err := db.Close(); err != nil {
		log.Printf("closing database connection: %v", err)
	}

	log.Println("stopped")
}

// validUsername returns a bool indicating the provided username is valid under
// the fmrl spec.
func validUsername(username string) bool {
	if len(username) > 40 || len(username) == 0 {
		// Usernames are limited to 40 characters/bytes
		return false
	}

	// From spec:
	// 		A valid username consists only of ASCII lowercase letters, numbers,
	//		and the following characters: _. All together, the valid character
	//		set is abcdefghijklmnopqrstuvwxyz0123456789_.
	for _, b := range []byte(username) {
		if b < 0x30 || (b >= 0x3A && b <= 0x60) || b >= 0x7B {
			return false
		}
	}
	return true
}

func passwordHash() int {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal(err)
		return 1
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	t := term.NewTerminal(os.Stdin, "> ")
	password, err := t.ReadLine()
	if err != nil {
		fmt.Fprintf(t, "%v", err)
		return 1
	}

	argon := argon2.DefaultConfig()
	encoded, err := argon.HashEncoded([]byte(password))
	if err != nil {
		fmt.Fprintf(t, "%v", err)
		return 1
	}

	t.Write(encoded)
	t.Write([]byte{'\n'})

	return 0
}
