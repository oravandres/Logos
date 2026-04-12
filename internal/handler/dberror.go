package handler

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	pgUniqueViolation = "23505"
	pgFKViolation     = "23503"
	pgCheckViolation  = "23514"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

func isFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgFKViolation
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgCheckViolation
}
