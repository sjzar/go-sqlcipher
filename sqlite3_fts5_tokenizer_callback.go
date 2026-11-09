// Copyright (C) 2026 The go-sqlcipher authors.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build cgo

package sqlite3

// Exported FTS5 tokenizer callbacks live in their own file because cgo forbids
// C definitions in the preamble of a file that uses //export.

/*
#include <stdlib.h>
*/
import "C"

import "unsafe"

//export gsc_fts5TokenizerCreate
func gsc_fts5TokenizerCreate(factory unsafe.Pointer, azArg **C.char, nArg C.int, ppOut *unsafe.Pointer) C.int {
	return C.int(fts5CreateTokenizer(factory, azArg, nArg, ppOut))
}

//export gsc_fts5TokenizerDelete
func gsc_fts5TokenizerDelete(tokenizer unsafe.Pointer) {
	deleteHandle(tokenizer)
}

//export gsc_fts5TokenizerTokenize
func gsc_fts5TokenizerTokenize(tokenizer unsafe.Pointer, pCtx unsafe.Pointer, flags C.int, pText *C.char, nText C.int, xToken unsafe.Pointer) C.int {
	return C.int(fts5Tokenize(tokenizer, pCtx, flags, pText, nText, xToken))
}

//export gsc_fts5TokenizerDestroy
func gsc_fts5TokenizerDestroy(factory unsafe.Pointer) {
	deleteHandle(factory)
}
