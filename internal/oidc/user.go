package oidc

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// FindOrCreateUser resolves an OIDC identity to a Know user.
//
// Strategy:
// 1. Look up by (provider, subject) -> found -> return user
// 2. Look up by email -> found -> link OIDC identity -> return user
// 3. If selfSignup enabled -> create new user -> return user
// 4. Else -> return error
func FindOrCreateUser(ctx context.Context, dbClient *db.Client, info *UserInfo, selfSignup bool) (*models.User, error) {
	// 1. Exact OIDC match
	user, err := dbClient.GetUserByOIDCSubject(ctx, info.Provider, info.Subject)
	if err != nil {
		return nil, fmt.Errorf("oidc user lookup: %w", err)
	}
	if user != nil {
		return user, nil
	}

	// 2. Email match -> link identity
	if info.Email != "" {
		user, err = dbClient.GetUserByEmail(ctx, info.Email)
		if err != nil {
			return nil, fmt.Errorf("email user lookup: %w", err)
		}
		if user != nil {
			uid, err := models.RecordIDString(user.ID)
			if err != nil {
				return nil, fmt.Errorf("extract user id: %w", err)
			}
			logutil.FromCtx(ctx).Info("linking OIDC identity to existing user via email match",
				"user_id", uid, "email", info.Email, "provider", info.Provider, "subject", info.Subject)
			if err := dbClient.LinkOIDCIdentity(ctx, uid, info.Provider, info.Subject); err != nil {
				return nil, fmt.Errorf("link oidc identity: %w", err)
			}
			return user, nil
		}
	}

	// 3. Self-signup
	if !selfSignup {
		return nil, fmt.Errorf("registration disabled: no matching user for %s", info.Email)
	}

	user, err = dbClient.CreateUserFromOIDC(ctx, info.Provider, info.Subject, info.Name, info.Email)
	if err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}

	return user, nil
}
