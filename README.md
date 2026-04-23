# pgtestlog

`pgtestlog` is a Go library for recording and inspecting changes in a PostgreSQL database, primarily designed for use in
integration tests. It uses PostgreSQL triggers to automatically log `INSERT`, `UPDATE`, and `DELETE` operations into a
dedicated change log table.

## Features

- **Automatic Setup**: Automatically creates a change log table and attaches triggers to all existing tables in the
  current schema.
- **Detailed Logs**: Captures the table name, operation type, primary key(s), full row data, and column-level diffs (for
  updates).
- **Composite Key Support**: Handles tables with composite primary keys.
- **ASCII Rendering**: Includes a built-in renderer to produce a human-readable, tree-like ASCII representation of
  database changes.
- **Transaction Aware**: Captures PostgreSQL transaction IDs (`tx_id`) for grouping related changes.

## Installation

```bash
go get github.com/conwayok/pgtestlog
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/conwayok/pgtestlog"
	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	connStr := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, `
		DROP TABLE IF EXISTS users;
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create table: %v\n", err)
	}

	recorder := pgtestlog.New(pgtestlog.Config{
		TablePrefix: "__debug",
	})

	err = recorder.Setup(ctx, conn)
	if err != nil {
		log.Fatalf("Failed to setup recorder: %v\n", err)
	}

	fmt.Println("Performing database operations...")

	_, err = conn.Exec(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", "Foo", "foo@example.com")
	if err != nil {
		log.Fatalf("Insert failed: %v\n", err)
	}

	_, err = conn.Exec(ctx, "UPDATE users SET name = $1 WHERE email = $2", "Foo Bar", "foo@example.com")
	if err != nil {
		log.Fatalf("Update failed: %v\n", err)
	}

	_, err = conn.Exec(ctx, "DELETE FROM users WHERE email = $1", "foo@example.com")
	if err != nil {
		log.Fatalf("Delete failed: %v\n", err)
	}

	logs, err := recorder.GetLogs(ctx, conn, []string{"users"})
	if err != nil {
		log.Fatalf("Failed to get logs: %v\n", err)
	}

	fmt.Println("Database changes captured:")
	output := pgtestlog.RenderAscii(logs)
	fmt.Println(output)
}
```

Output:

```
Performing database operations...
Database changes captured:
Tx 757 @ 2026-04-23T21:20:33+08:00
 ├── INSERT
 │    └── users
 │         ├── id : 1
 │         ├── name : Foo
 │         ├── email : foo@example.com

Tx 758 @ 2026-04-23T21:20:33+08:00
 ├── UPDATE
 │    └── users
 │         └── [PK] : id=1
 │         └── name : "Foo" -> "Foo Bar"

Tx 759 @ 2026-04-23T21:20:33+08:00
 ├── DELETE
 │    └── users
 │         └── [PK] : id=1
 │         ├── id : 1
 │         ├── name : Foo Bar
 │         ├── email : foo@example.com

```

## How it Works

When `Setup` is called, `pgtestlog` does the following:

1. Creates a table (default name: `__debug_change_logs`) to store the history.
2. Creates a PL/pgSQL trigger function that captures changes.
3. Iterates through all tables in the `public` (or current) schema and attaches an `AFTER INSERT OR UPDATE OR DELETE`
   trigger to each one.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.