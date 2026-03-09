package graph

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

// seedIfEmpty checks each bootstrap resource (user, vault, membership)
// independently and creates any that are missing. This allows the server to
// self-bootstrap against an empty database (e.g. when running as a launchd
// service) and to recover from a partial previous bootstrap.
func seedIfEmpty(ctx context.Context, dbClient *db.Client) error {
	// 1. Ensure admin user exists
	user, err := dbClient.GetUser(ctx, "admin")
	if err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}
	if user == nil {
		slog.Info("auto-bootstrap: creating admin user")
		user, err = dbClient.CreateUserWithID(ctx, "admin", models.UserInput{
			Name: "admin",
		})
		if err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		return fmt.Errorf("extract user id: %w", err)
	}

	if !user.IsSystemAdmin {
		if err := dbClient.UpdateUserSystemAdmin(ctx, userID, true); err != nil {
			return fmt.Errorf("set system admin: %w", err)
		}
	}

	// 2. Ensure default vault exists
	v, err := dbClient.GetVault(ctx, "default")
	if err != nil {
		return fmt.Errorf("check default vault: %w", err)
	}
	if v == nil {
		slog.Info("auto-bootstrap: creating default vault")
		desc := "Default vault"
		v, err = dbClient.CreateVaultWithID(ctx, "default", userID, models.VaultInput{
			Name:        "default",
			Description: &desc,
		})
		if err != nil {
			return fmt.Errorf("create default vault: %w", err)
		}
	}

	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		return fmt.Errorf("extract vault id: %w", err)
	}

	// 3. Ensure vault membership exists
	members, err := dbClient.GetVaultMembers(ctx, vaultID)
	if err != nil {
		return fmt.Errorf("check vault members: %w", err)
	}
	hasMembership := false
	for _, m := range members {
		mid, midErr := models.RecordIDString(m.User)
		if midErr == nil && mid == userID {
			hasMembership = true
			break
		}
	}
	if !hasMembership {
		slog.Info("auto-bootstrap: creating vault membership")
		if _, err := dbClient.CreateVaultMember(ctx, userID, vaultID, models.RoleAdmin); err != nil {
			return fmt.Errorf("create vault member: %w", err)
		}
	}

	slog.Info("auto-bootstrap complete", "user", userID, "vault", vaultID)
	return nil
}
