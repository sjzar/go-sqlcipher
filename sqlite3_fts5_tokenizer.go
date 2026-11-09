// Copyright (C) 2026 The go-sqlcipher authors.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build cgo

package sqlite3

/*
#ifndef USE_LIBSQLITE3
#include "sqlite3-binding.h"
#else
#include <sqlite3.h>
#endif
#include <stdlib.h>

extern int gsc_fts5TokenizerCreate(void *factory, char **azArg, int nArg, void **ppOut);
extern void gsc_fts5TokenizerDelete(void *tokenizer);
extern int gsc_fts5TokenizerTokenize(void *tokenizer, void *pCtx, int flags, char *pText, int nText, void *xToken);
extern void gsc_fts5TokenizerDestroy(void *factory);

static int
_sqlite3_fts5_x_create(void *pUserData, const char **azArg, int nArg, Fts5Tokenizer **ppOut) {
  return gsc_fts5TokenizerCreate(pUserData, (char **)azArg, nArg, (void **)ppOut);
}

static void
_sqlite3_fts5_x_delete(Fts5Tokenizer *p) {
  gsc_fts5TokenizerDelete((void *)p);
}

static int
_sqlite3_fts5_x_tokenize(Fts5Tokenizer *p, void *pCtx, int flags, const char *pText, int nText,
    int (*xToken)(void *, int, const char *, int, int, int)) {
  return gsc_fts5TokenizerTokenize((void *)p, pCtx, flags, (char *)pText, nText, (void *)xToken);
}

static void
_sqlite3_fts5_x_destroy(void *pUserData) {
  gsc_fts5TokenizerDestroy(pUserData);
}

static fts5_tokenizer _sqlite3_fts5_tokenizer_impl = {
  _sqlite3_fts5_x_create, _sqlite3_fts5_x_delete, _sqlite3_fts5_x_tokenize,
};

static int
_sqlite3_fts5_emit(void *xToken, void *pCtx, int tflags, const char *pToken, int nToken, int iStart, int iEnd) {
  return ((int (*)(void *, int, const char *, int, int, int))xToken)(pCtx, tflags, pToken, nToken, iStart, iEnd);
}

// Looks up fts5_api on this connection and registers the Go-backed tokenizer.
// The caller keeps ownership of pUserData only when this returns an error.
static int
_sqlite3_fts5_create_tokenizer(sqlite3 *db, const char *zName, void *pUserData) {
  fts5_api *api = 0;
  sqlite3_stmt *stmt = 0;
  int rc = sqlite3_prepare_v2(db, "SELECT fts5(?1)", -1, &stmt, 0);
  if (rc != SQLITE_OK) {
    return rc;
  }
  sqlite3_bind_pointer(stmt, 1, (void *)&api, "fts5_api_ptr", 0);
  sqlite3_step(stmt);
  rc = sqlite3_finalize(stmt);
  if (rc != SQLITE_OK) {
    return rc;
  }
  if (api == 0) {
    return SQLITE_ERROR;
  }
  return api->xCreateTokenizer(api, zName, pUserData, &_sqlite3_fts5_tokenizer_impl, _sqlite3_fts5_x_destroy);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

type fts5TokenizerFactory struct {
	create func(args []string) (FTS5Tokenizer, error)
}

// RegisterFTS5Tokenizer registers a Go tokenizer under name on this connection.
//
// FTS5 tables reference it with tokenize = 'name arg1 arg2'; create receives the
// arguments after the name each time FTS5 instantiates the tokenizer. Register
// it on every pool connection, for example from SQLiteDriver.ConnectHook. For
// encrypted databases the key must already be applied, which holds for DSN key
// pragmas because the driver executes them before ConnectHook. Registering the
// same name again replaces it for tables opened afterwards. Callbacks are
// released when the connection closes.
func (c *SQLiteConn) RegisterFTS5Tokenizer(name string, create func(args []string) (FTS5Tokenizer, error)) error {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return errors.New("sqlite3: invalid FTS5 tokenizer name")
	}
	if create == nil {
		return errors.New("sqlite3: nil FTS5 tokenizer constructor")
	}
	if !c.dbConnOpen() {
		return errors.New("sqlite3: connection is closed")
	}
	handle := newHandle(c, &fts5TokenizerFactory{create: create})
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	if rv := C._sqlite3_fts5_create_tokenizer(c.db, cname, handle); rv != C.SQLITE_OK {
		deleteHandle(handle)
		if err := c.lastError(); err != nil {
			return err
		}
		return fmt.Errorf("sqlite3: register FTS5 tokenizer %q: result code %d", name, int(rv))
	}
	return nil
}

// fts5TokenStopped carries a non-OK result returned by FTS5's xToken callback.
type fts5TokenStopped struct{ rc C.int }

func (e fts5TokenStopped) Error() string {
	return fmt.Sprintf("sqlite3: FTS5 token callback returned %d", int(e.rc))
}

func fts5Tokenize(tokenizer unsafe.Pointer, pCtx unsafe.Pointer, flags C.int, pText *C.char, nText C.int, xToken unsafe.Pointer) (rc C.int) {
	tok, ok := lookupHandle(tokenizer).(FTS5Tokenizer)
	if !ok {
		return C.SQLITE_ERROR
	}
	text := unsafe.Slice((*byte)(unsafe.Pointer(pText)), int(nText))
	stopped := C.int(C.SQLITE_OK)
	emit := func(token []byte, start, end int, colocated bool) error {
		if stopped != C.SQLITE_OK {
			return fts5TokenStopped{rc: stopped}
		}
		if len(token) == 0 || len(token) > 1<<30 {
			return errors.New("sqlite3: FTS5 token must be non-empty")
		}
		if start < 0 || end < start || end > len(text) {
			return fmt.Errorf("sqlite3: FTS5 token offsets [%d,%d) outside text of %d bytes", start, end, len(text))
		}
		var tflags C.int
		if colocated {
			tflags = 1 // FTS5_TOKEN_COLOCATED
		}
		if rv := C._sqlite3_fts5_emit(xToken, pCtx, tflags, (*C.char)(unsafe.Pointer(&token[0])), C.int(len(token)), C.int(start), C.int(end)); rv != C.SQLITE_OK {
			stopped = rv
			return fts5TokenStopped{rc: rv}
		}
		return nil
	}
	defer func() {
		if recover() != nil {
			rc = C.SQLITE_ERROR
		}
	}()
	err := tok.Tokenize(text, int(flags), emit)
	if stopped != C.SQLITE_OK {
		return stopped
	}
	if err != nil {
		return C.SQLITE_ERROR
	}
	return C.SQLITE_OK
}

func fts5CreateTokenizer(factory unsafe.Pointer, azArg **C.char, nArg C.int, ppOut *unsafe.Pointer) (rc C.int) {
	hv := lookupHandleVal(factory)
	f, ok := hv.val.(*fts5TokenizerFactory)
	if !ok {
		return C.SQLITE_ERROR
	}
	defer func() {
		if recover() != nil {
			rc = C.SQLITE_ERROR
		}
	}()
	argv := unsafe.Slice(azArg, int(nArg))
	args := make([]string, len(argv))
	for i, arg := range argv {
		args[i] = C.GoString(arg)
	}
	tok, err := f.create(args)
	if err != nil || tok == nil {
		return C.SQLITE_ERROR
	}
	*ppOut = newHandle(hv.db, tok)
	return C.SQLITE_OK
}
