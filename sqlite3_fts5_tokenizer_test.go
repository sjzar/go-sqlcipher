// Copyright (C) 2026 The go-sqlcipher authors.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

//go:build cgo

package sqlite3

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// FTS5 必须默认编入，不依赖 sqlite_fts5 构建标签。
func TestFTS5EnabledByDefault(t *testing.T) {
	db, err := sql.Open("sqlcipher", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body)`); err != nil {
		t.Fatalf("create fts5 table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO docs(body) VALUES ('hello world'), ('other text')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM docs WHERE docs MATCH 'world'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("match count = %d, want 1", n)
	}
}

// spaceTokenizer 按 ASCII 空格切分并转小写；写入文档时，synonyms 中的词额外输出同位 token。
type spaceTokenizer struct {
	args     []string
	synonyms map[string]string
	fail     error
	panicMsg string
	mu       *sync.Mutex
	flags    *[]int
}

func (s *spaceTokenizer) Tokenize(text []byte, flags int, emit FTS5TokenEmitter) error {
	if s.mu != nil {
		s.mu.Lock()
		*s.flags = append(*s.flags, flags)
		s.mu.Unlock()
	}
	if s.panicMsg != "" {
		panic(s.panicMsg)
	}
	if s.fail != nil {
		return s.fail
	}
	start := -1
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] != ' ' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start < 0 {
			continue
		}
		word := bytes.ToLower(text[start:i])
		if err := emit(word, start, i, false); err != nil {
			return err
		}
		if syn, ok := s.synonyms[string(word)]; ok && flags&FTS5TokenizeDocument != 0 {
			if err := emit([]byte(syn), start, i, true); err != nil {
				return err
			}
		}
		start = -1
	}
	return nil
}

// openTokenizerDB 打开只含一个连接的库，并在每个连接上注册 create。
func openTokenizerDB(t *testing.T, dsn, name string, create func([]string) (FTS5Tokenizer, error)) *sql.DB {
	t.Helper()
	driverName := "sqlcipher_fts5_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	sql.Register(driverName, &SQLiteDriver{ConnectHook: func(c *SQLiteConn) error {
		return c.RegisterFTS5Tokenizer(name, create)
	}})
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFTS5TokenizerDocumentQueryAndOffsets(t *testing.T) {
	var mu sync.Mutex
	var flags []int
	var gotArgs []string
	db := openTokenizerDB(t, ":memory:", "space", func(args []string) (FTS5Tokenizer, error) {
		gotArgs = slices.Clone(args)
		return &spaceTokenizer{args: args, mu: &mu, flags: &flags}, nil
	})
	if _, err := db.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = 'space alpha beta')`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !slices.Equal(gotArgs, []string{"alpha", "beta"}) {
		t.Fatalf("tokenizer args = %q, want [alpha beta]", gotArgs)
	}
	if _, err := db.Exec(`INSERT INTO docs(body) VALUES ('Hello World'), ('other text')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var body string
	if err := db.QueryRow(`SELECT highlight(docs, 0, '[', ']') FROM docs WHERE docs MATCH 'world'`).Scan(&body); err != nil {
		t.Fatalf("match: %v", err)
	}
	if body != "Hello [World]" {
		t.Fatalf("highlight = %q, want %q", body, "Hello [World]")
	}
	mu.Lock()
	defer mu.Unlock()
	var sawDocument, sawQuery, sawAux bool
	for _, f := range flags {
		sawDocument = sawDocument || f&FTS5TokenizeDocument != 0
		sawQuery = sawQuery || f&FTS5TokenizeQuery != 0
		sawAux = sawAux || f&FTS5TokenizeAux != 0
	}
	if !sawDocument || !sawQuery || !sawAux {
		t.Fatalf("flags = %v, want document, query and aux calls", flags)
	}
}

func TestFTS5TokenizerColocatedSynonym(t *testing.T) {
	db := openTokenizerDB(t, ":memory:", "space", func([]string) (FTS5Tokenizer, error) {
		return &spaceTokenizer{synonyms: map[string]string{"one": "1"}}, nil
	})
	if _, err := db.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = 'space')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO docs(body) VALUES ('one apple'), ('two apples')`); err != nil {
		t.Fatal(err)
	}
	// 同位 token 不占新位置：原词与同义词都必须和后一个词相邻。
	for _, phrase := range []string{`'"one apple"'`, `'"1 apple"'`} {
		var body string
		if err := db.QueryRow(`SELECT body FROM docs WHERE docs MATCH ` + phrase).Scan(&body); err != nil {
			t.Fatalf("phrase %s: %v", phrase, err)
		}
		if body != "one apple" {
			t.Fatalf("phrase %s body = %q, want %q", phrase, body, "one apple")
		}
	}
}

func TestFTS5TokenizerErrors(t *testing.T) {
	createErr := errors.New("bad tokenizer arguments")
	db := openTokenizerDB(t, ":memory:", "space", func(args []string) (FTS5Tokenizer, error) {
		if slices.Contains(args, "reject") {
			return nil, createErr
		}
		if slices.Contains(args, "panic") {
			return &spaceTokenizer{panicMsg: "boom"}, nil
		}
		if slices.Contains(args, "fail") {
			return &spaceTokenizer{fail: errors.New("tokenize failed")}, nil
		}
		return &spaceTokenizer{}, nil
	})
	if _, err := db.Exec(`CREATE VIRTUAL TABLE rejected USING fts5(body, tokenize = 'space reject')`); err == nil {
		t.Fatal("constructor error was not reported")
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE failing USING fts5(body, tokenize = 'space fail')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO failing(body) VALUES ('x')`); err == nil {
		t.Fatal("tokenize error was not reported")
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE panicking USING fts5(body, tokenize = 'space panic')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO panicking(body) VALUES ('x')`); err == nil {
		t.Fatal("tokenize panic was not reported as an error")
	}
	// 连接在错误之后仍可用。
	if _, err := db.Exec(`CREATE VIRTUAL TABLE ok USING fts5(body, tokenize = 'space')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ok(body) VALUES ('still works')`); err != nil {
		t.Fatal(err)
	}
}

func TestFTS5TokenizerEmitRejectsInvalidToken(t *testing.T) {
	cases := map[string]func([]byte, FTS5TokenEmitter) error{
		"empty token":    func(text []byte, emit FTS5TokenEmitter) error { return emit(nil, 0, 0, false) },
		"negative start": func(text []byte, emit FTS5TokenEmitter) error { return emit([]byte("x"), -1, 1, false) },
		"end past text":  func(text []byte, emit FTS5TokenEmitter) error { return emit([]byte("x"), 0, len(text)+1, false) },
		"start after end": func(text []byte, emit FTS5TokenEmitter) error {
			return emit([]byte("x"), 1, 0, false)
		},
	}
	for name, tokenize := range cases {
		t.Run(name, func(t *testing.T) {
			var emitErr error
			db := openTokenizerDB(t, ":memory:", "custom", func([]string) (FTS5Tokenizer, error) {
				return fts5TokenizerFunc(func(text []byte, flags int, emit FTS5TokenEmitter) error {
					emitErr = tokenize(text, emit)
					return emitErr
				}), nil
			})
			if _, err := db.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = 'custom')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO docs(body) VALUES ('ab')`); err == nil {
				t.Fatal("invalid token was accepted")
			}
			if emitErr == nil {
				t.Fatal("emit did not return an error")
			}
		})
	}
}

type fts5TokenizerFunc func(text []byte, flags int, emit FTS5TokenEmitter) error

func (f fts5TokenizerFunc) Tokenize(text []byte, flags int, emit FTS5TokenEmitter) error {
	return f(text, flags, emit)
}

func TestRegisterFTS5TokenizerRejectsInvalidArguments(t *testing.T) {
	db, err := sql.Open("sqlcipher", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn any) error {
		c := driverConn.(*SQLiteConn)
		create := func([]string) (FTS5Tokenizer, error) { return &spaceTokenizer{}, nil }
		if err := c.RegisterFTS5Tokenizer("", create); err == nil {
			return errors.New("empty name accepted")
		}
		if err := c.RegisterFTS5Tokenizer("x\x00y", create); err == nil {
			return errors.New("name with NUL accepted")
		}
		if err := c.RegisterFTS5Tokenizer("space", nil); err == nil {
			return errors.New("nil constructor accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegisterFTS5TokenizerOnClosedConnection(t *testing.T) {
	driverConn, err := (&SQLiteDriver{}).Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	c := driverConn.(*SQLiteConn)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	err = c.RegisterFTS5Tokenizer("space", func([]string) (FTS5Tokenizer, error) { return &spaceTokenizer{}, nil })
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("registration on a closed connection: err = %v, want connection closed", err)
	}
}

// 加密库：DSN 中的密钥在 ConnectHook 之前生效，注册与读写都在密钥之后进行。
func TestFTS5TokenizerOnEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fts.db")
	key := "x'" + strings.Repeat("ab", 32) + "'"
	dsn := "file:" + path + "?_pragma_key=" + key + "&_pragma_cipher_page_size=4096"
	create := func([]string) (FTS5Tokenizer, error) { return &spaceTokenizer{}, nil }

	db := openTokenizerDB(t, dsn, "space", create)
	if _, err := db.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = 'space')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO docs(body) VALUES ('Secret Plan')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	sql.Register("sqlcipher_fts5_reopen", &SQLiteDriver{ConnectHook: func(c *SQLiteConn) error {
		return c.RegisterFTS5Tokenizer("space", create)
	}})
	reopened, err := sql.Open("sqlcipher_fts5_reopen", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var body string
	if err := reopened.QueryRow(`SELECT body FROM docs WHERE docs MATCH 'plan'`).Scan(&body); err != nil {
		t.Fatalf("match after reopen: %v", err)
	}
	if body != "Secret Plan" {
		t.Fatalf("body = %q", body)
	}
}

// 连接关闭后，注册时和建表时创建的 handle 必须全部释放（按全局数量比较，覆盖未绑定连接的 handle）。
func TestFTS5TokenizerReleasesHandlesOnClose(t *testing.T) {
	countHandles := func(*SQLiteConn) int {
		n := 0
		handleVals.Range(func(_, _ any) bool {
			n++
			return true
		})
		return n
	}
	baseline := countHandles(nil)
	d := &SQLiteDriver{}
	driverConn, err := d.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	c := driverConn.(*SQLiteConn)
	if err := c.RegisterFTS5Tokenizer("space", func([]string) (FTS5Tokenizer, error) { return &spaceTokenizer{}, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(`CREATE VIRTUAL TABLE docs USING fts5(body, tokenize = 'space')`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(`INSERT INTO docs(body) VALUES ('a b')`, nil); err != nil {
		t.Fatal(err)
	}
	if countHandles(c) <= baseline {
		t.Fatal("expected live handles before close")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if n := countHandles(c); n != baseline {
		t.Fatalf("%d handles leaked after close", n-baseline)
	}
}
