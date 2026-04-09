package model

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// OptionalUUID is a nullable UUID pointer used for optional foreign keys.
type OptionalUUID = *uuid.UUID

func uuidFromPgtype(p pgtype.UUID) uuid.UUID {
	return uuid.UUID(p.Bytes)
}

// UUIDToPgtype converts a google/uuid.UUID into a pgtype.UUID.
func UUIDToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// OptionalUUIDToPgtype converts a nullable UUID pointer into a pgtype.UUID.
func OptionalUUIDToPgtype(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// OptionalStringToPgtext converts a nullable string pointer into a pgtype.Text.
func OptionalStringToPgtext(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// StringToPgtext converts a string into a pgtype.Text; empty strings become NULL.
func StringToPgtext(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
