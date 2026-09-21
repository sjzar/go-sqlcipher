## go-sqlcipher

### Note

This project is modified from [mutecomm/go-sqlcipher](https://github.com/mutecomm/go-sqlcipher) and [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3). It has not been rigorously tested, so please do not use it in a production environment.

### Description

Self-contained Go sqlite3 driver with an AES-256 encrypted sqlite3 database
conforming to the built-in database/sql interface. It is based on:

- Go sqlite3 driver: https://github.com/sjzar/go-sqlcipher
- SQLite extension with AES-256 codec: https://github.com/sqlcipher/sqlcipher
- AES-256 implementation from: https://github.com/libtom/libtomcrypt

SQLite itself is part of SQLCipher.

The Go driver tracks **mattn/go-sqlite3 v1.14.52** and requires **Go 1.21+**
and CGO. The bundled encryption engine remains **SQLCipher 4.12.0 / SQLite
3.51.1**; updating the Go driver does not replace the encryption engine.

### Driver name and coexistence

The default `database/sql` driver name is now **`sqlcipher`**. Existing callers
must change `sql.Open("sqlite3", dsn)` to `sql.Open("sqlcipher", dsn)` when
upgrading from v0.0.3. The Go package name and exported API remain `sqlite3`.

SQLite C API symbols, C bridge helpers, and exported Go callbacks use a `gsc_`
prefix, so this package can coexist with unmodified `mattn/go-sqlite3`, which
keeps the `sqlite3` driver name. Database handles, callback registrations and
allocated memory belong to their originating driver and must not be mixed.

`driverName` remains overridable with `-ldflags -X` for existing custom-name
builds. Using `sqlite3` as the override is only valid when mattn is absent.
There is no automatic `sqlite3` alias because it would recreate the conflict.

### Incompatibilities of SQLCipher

The module's version tags are independent of the bundled SQLCipher version.

**SQLCipher 4.x is incompatible with SQLCipher 3.x!**

go-sqlcipher does not implement any migration strategies at the moment.
So if you upgrade a major version of go-sqlcipher, you yourself are responsible
to upgrade existing database files.

See [migrating databases](https://www.zetetic.net/sqlcipher/sqlcipher-api/#Migrating_Databases) for details.

To upgrade your Go code to the 4.x series, change the import path to

    "github.com/sjzar/go-sqlcipher"

### Installation

This package can be installed with the go get command:

    go get github.com/sjzar/go-sqlcipher


### Documentation

To create and open encrypted database files use the following DSN parameters:

```go
key := "2DD29CA851E7B56E4697B0E1F08507293D761A05CE4D1B628663F411A8086D99"
dbname := fmt.Sprintf("db?_pragma_key=x'%s'&_pragma_cipher_page_size=4096", key)
db, _ := sql.Open("sqlcipher", dbname)
```

`_pragma_key` is the hex encoded 32 byte key (must be 64 characters long).
`_pragma_cipher_page_size` is the page size of the encrypted database (set if
you want a different value than the default size).

```go
key := url.QueryEscape("secret")
dbname := fmt.Sprintf("db?_pragma_key=%s&_pragma_cipher_page_size=4096", key)
db, _ := sql.Open("sqlcipher", dbname)
```

This uses a passphrase directly as `_pragma_key` with the key derivation function in
SQLCipher. Do not forget the `url.QueryEscape()` call in your code!

See also [PRAGMA key](https://www.zetetic.net/sqlcipher/sqlcipher-api/#PRAGMA_key).

API documentation can be found here:
http://godoc.org/github.com/sjzar/go-sqlcipher

Use the function
[sqlite3.IsEncrypted()](https://godoc.org/github.com/sjzar/go-sqlcipher#IsEncrypted)
to check whether a database file is encrypted or not.

Examples can be found under the `./_example` directory

### Development

Run `make test` for default and optional-feature race tests and `go vet`.
Run `go generate ./...` after updating the C sources or bridge helpers, and
commit the generated namespace header. See [MAINTENANCE](MAINTENANCE) for the
upstream synchronization procedure and the patches that must be preserved.

The imported upstream code still has existing Staticcheck findings (including
unused conversion helpers, deprecated test imports, and error-string style);
`make test` does not claim a clean Staticcheck result.


### License

The code of the originating packages is covered by their respective licenses.
See [LICENSE](LICENSE) file for details.
