// Package db provides embedded database schema and migration files.
package db

import _ "embed"

// Schema contains the DDL statements for all application tables.
//
//go:embed migrations/001_schema.sql
var Schema string
