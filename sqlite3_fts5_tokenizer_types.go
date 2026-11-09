// Copyright (C) 2026 The go-sqlcipher authors.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package sqlite3

// Tokenize flags passed to FTS5Tokenizer.Tokenize. They mirror the
// FTS5_TOKENIZE_* values in the FTS5 C API.
const (
	// FTS5TokenizeQuery marks text from a MATCH expression or fts5vocab query.
	FTS5TokenizeQuery = 0x0001
	// FTS5TokenizePrefix is set together with FTS5TokenizeQuery for a prefix query term.
	FTS5TokenizePrefix = 0x0002
	// FTS5TokenizeDocument marks document text being inserted into or deleted from the index.
	FTS5TokenizeDocument = 0x0004
	// FTS5TokenizeAux marks text tokenized by auxiliary functions such as highlight and snippet.
	FTS5TokenizeAux = 0x0008
)

// FTS5TokenEmitter reports one token to FTS5. token must be non-empty, and
// start/end are byte offsets into the text given to Tokenize with
// 0 <= start <= end <= len(text). colocated marks a synonym that occupies the
// same position as the previous token. A non-nil error means FTS5 asked to
// stop; Tokenize should return it unchanged.
type FTS5TokenEmitter func(token []byte, start, end int, colocated bool) error

// FTS5Tokenizer is a custom FTS5 tokenizer implemented in Go.
//
// text is only valid during the call and must not be retained or modified.
// flags is a combination of the FTS5Tokenize* constants. Returning an error
// makes the SQL statement that triggered tokenization fail.
type FTS5Tokenizer interface {
	Tokenize(text []byte, flags int, emit FTS5TokenEmitter) error
}
