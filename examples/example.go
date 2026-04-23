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
