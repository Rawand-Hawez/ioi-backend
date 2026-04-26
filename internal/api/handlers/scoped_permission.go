package handlers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"IOI-real-estate-backend/internal/db"
)

var errScopedPermissionDenied = errors.New("permission denied for requested scope")

func requireScopedPermission(
	ctx context.Context,
	q *db.Queries,
	userID uuid.UUID,
	permissionKey string,
	businessEntityID pgtype.UUID,
	branchID pgtype.UUID,
	projectID pgtype.UUID,
) error {
	ok, err := q.CheckUserPermissionInScope(ctx, db.CheckUserPermissionInScopeParams{
		UserID:    toPgUUID(userID),
		Key:       permissionKey,
		ScopeID:   businessEntityID,
		ScopeID_2: branchID,
		ScopeID_3: projectID,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errScopedPermissionDenied
	}
	return nil
}
