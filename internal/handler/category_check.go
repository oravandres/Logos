package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/model"
)

var errCategoryTypeMismatch = errors.New("category type mismatch")

// validateCategoryType looks up a category by ID and verifies its type matches expectedType.
func validateCategoryType(ctx context.Context, q *dbq.Queries, categoryID uuid.UUID, expectedType string) error {
	cat, err := q.GetCategory(ctx, model.UUIDToPgtype(categoryID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("referenced category does not exist")
		}
		return fmt.Errorf("validate category: %w", err)
	}
	if cat.Type != expectedType {
		return fmt.Errorf("%w: expected %q, got %q", errCategoryTypeMismatch, expectedType, cat.Type)
	}
	return nil
}

// writeCategoryTypeError sends the appropriate HTTP error for a category validation failure.
func writeCategoryTypeError(w http.ResponseWriter, expectedType string, err error) {
	if errors.Is(err, errCategoryTypeMismatch) {
		respondError(w, http.StatusUnprocessableEntity, fmt.Sprintf("category type must be %q", expectedType))
		return
	}
	status := http.StatusInternalServerError
	msg := "failed to validate category"
	if err.Error() == "referenced category does not exist" {
		status = http.StatusUnprocessableEntity
		msg = err.Error()
	}
	respondError(w, status, msg)
}
