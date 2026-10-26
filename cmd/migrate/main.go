// Command tenttiarkisto-migrate applies migrations, seeds example data
// and manages the super-admin user.
//
//	go run ./cmd/migrate                 # apply migrations
//	go run ./cmd/migrate -seed           # ... + seed example subjects
//	go run ./cmd/migrate -admin-email a@b.fi -admin-password x -admin-org "Org"
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/JaniL/tenttiarkisto/internal/models"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	var (
		seed      = flag.Bool("seed", false, "seed example subjects (idempotent)")
		adminMail = flag.String("admin-email", "", "create/promote super admin with this email")
		adminPass = flag.String("admin-password", "", "super admin password")
		adminOrg  = flag.String("admin-org", "", "super admin organization")
	)
	flag.Parse()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://localhost:5432/tenttiarkisto?sslmode=disable"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	pool, err := db.Connect(ctx, databaseURL, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db connect:", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Println("database up to date")

	store := models.NewStore(pool)

	if *seed {
		if err := seedSubjects(ctx, store); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
			os.Exit(1)
		}
		fmt.Println("seed applied")
	}

	if *adminMail != "" {
		if *adminPass == "" || *adminOrg == "" {
			fmt.Fprintln(os.Stderr, "-admin-password and -admin-org are required with -admin-email")
			os.Exit(1)
		}
		if err := upsertSuperAdmin(ctx, store, *adminMail, *adminPass, *adminOrg); err != nil {
			fmt.Fprintln(os.Stderr, "admin:", err)
			os.Exit(1)
		}
		fmt.Println("super admin ready:", *adminMail)
	}
}

func seedSubjects(ctx context.Context, store *models.Store) error {
	names := []string{
		"Matematiikka ja tilastotiede",
		"Tietojenkäsittelytiede",
		"Fysiikka",
		"Kemia",
	}
	for _, n := range names {
		if _, err := store.CreateSubjectIfMissing(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

func upsertSuperAdmin(ctx context.Context, store *models.Store, email, password, org string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u, err := store.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		return store.UpdateUser(ctx, u.ID, org, email, strPtr(string(hash)), u.SubjectIDs)
	case errors.Is(err, db.ErrNotFound):
		_, err := store.CreateUser(ctx, org, email, string(hash), true, nil)
		return err
	default:
		return err
	}
}

func strPtr(s string) *string { return &s }
