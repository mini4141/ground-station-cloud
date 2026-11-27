// Package db embeds the goose migrations; sqlc output lives in the sqlc subpackage.
package db

import "embed"

// Migrations are the goose SQL migrations.
//
//go:embed migrations/*.sql
var Migrations embed.FS
