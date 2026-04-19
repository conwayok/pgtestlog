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
	"log"

	"github.com/conwayok/pgtestlog"
	"github.com/jackc/pgx/v5"
)

func main() {

	recorder := pgtestlog.New(
		pgtestlog.Config{
			// optional prefix
			TablePrefix: "__debug",
		},
	)

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, "connection string")

	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close(ctx)

	err = recorder.Setup(ctx, conn)

	if err != nil {
		log.Fatal(err)
	}

	// do DB stuff

	logs, err := recorder.GetLogs(ctx, conn, []string{"users"})

	if err != nil {
		log.Fatal(err)
	}

	pgtestlog.RenderAscii(logs)
}
```

## How it Works

When `Setup` is called, `pgtestlog` does the following:

1. Creates a table (default name: `__debug_change_logs`) to store the history.
2. Creates a PL/pgSQL trigger function that captures changes.
3. Iterates through all tables in the `public` (or current) schema and attaches an `AFTER INSERT OR UPDATE OR DELETE`
   trigger to each one.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.