package main

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|version|force>")
	}
	cmd := os.Args[1]

	mongoURI := os.Getenv("MONGO_URI")
	dbName := os.Getenv("MONGO_DATABASE")
	if mongoURI == "" || dbName == "" {
		log.Fatal("MONGO_URI and MONGO_DATABASE env required")
	}

	dsn, err := buildDSN(mongoURI, dbName)
	if err != nil {
		log.Fatalf("build dsn: %v", err)
	}

	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("up: %v", err)
		}
	case "down":
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("down: %v", err)
		}
	case "version":
		v, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("version: %v", err)
		}
		log.Printf("version=%d dirty=%v", v, dirty)
		return
	case "force":
		if len(os.Args) < 3 {
			log.Fatal("usage: migrate force <version>")
		}
		var v int
		if _, err := fmt.Sscanf(os.Args[2], "%d", &v); err != nil {
			log.Fatalf("force: invalid version: %v", err)
		}
		if err := m.Force(v); err != nil {
			log.Fatalf("force: %v", err)
		}
	default:
		log.Fatalf("unknown command: %s", cmd)
	}
	log.Printf("migrate %s: ok", cmd)
}

// buildDSN injects the database name as path on the mongo URI as required
// by golang-migrate's mongodb driver, preserving any user-supplied options.
func buildDSN(uri, db string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	u.Path = "/" + db
	return u.String(), nil
}
