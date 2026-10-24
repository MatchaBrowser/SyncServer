// Package migrations holds the database migrations, written in Go so that each
// one can branch on the dialect (sqlite or postgres).
//
// A migration file is named <YYYYMMDDHHMMSS>_<name>.go and registers itself
// in init() with goose.AddMigrationContext. The dialect is read from the
// "dbDialect" value of the context.
package migrations
