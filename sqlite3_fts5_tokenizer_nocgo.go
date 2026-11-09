// Copyright (C) 2026 The go-sqlcipher authors.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build !cgo

package sqlite3

// RegisterFTS5Tokenizer requires cgo; this stub keeps CGO_ENABLED=0 builds compiling.
func (c *SQLiteConn) RegisterFTS5Tokenizer(string, func([]string) (FTS5Tokenizer, error)) error {
	return errorMsg
}
