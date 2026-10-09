package migrations

import _ "embed"

//go:embed 001_community.sql
var Schema string
