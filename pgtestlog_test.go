package pgtestlog

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestRenderAscii(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	tx1 := int64(1001)
	tx2 := int64(1002)

	tests := []struct {
		name     string
		logs     []Log
		contains []string
	}{
		{
			name:     "empty logs",
			logs:     []Log{},
			contains: []string{""},
		},
		{
			name: "single insert",
			logs: []Log{
				{
					TableName: "users",
					Operation: "INSERT",
					RowData:   map[string]any{"id": 1, "name": "foo"},
					TxId:      &tx1,
					ChangedAt: now,
				},
			},
			contains: []string{
				"Tx 1001 @ 2024-01-01T12:00:00Z",
				"├── INSERT",
				"│    └── users",
				"id : 1",
				"name : foo",
			},
		},
		{
			name: "single delete",
			logs: []Log{
				{
					TableName: "users",
					Operation: "DELETE",
					Pk:        map[string]any{"id": 1},
					RowData:   map[string]any{"id": 1, "name": "foo"},
					TxId:      &tx1,
					ChangedAt: now,
				},
			},
			contains: []string{
				"Tx 1001 @ 2024-01-01T12:00:00Z",
				"├── DELETE",
				"│    └── users",
				"└── [PK] : id=1",
				"id : 1",
				"name : foo",
			},
		},
		{
			name: "single update",
			logs: []Log{
				{
					TableName: "users",
					Operation: "UPDATE",
					Pk:        map[string]any{"id": 1},
					Diffs: map[string]any{
						"name": map[string]any{"old": "foo", "new": "bob"},
					},
					TxId:      &tx1,
					ChangedAt: now,
				},
			},
			contains: []string{
				"Tx 1001 @ 2024-01-01T12:00:00Z",
				"├── UPDATE",
				"│    └── users",
				"└── [PK] : id=1",
				"└── name : \"foo\" -> \"bob\"",
			},
		},
		{
			name: "multiple transactions",
			logs: []Log{
				{
					TableName: "users",
					Operation: "INSERT",
					RowData:   map[string]any{"id": 1, "name": "foo"},
					TxId:      &tx1,
					ChangedAt: now,
				},
				{
					TableName: "users",
					Operation: "UPDATE",
					Pk:        map[string]any{"id": 1},
					Diffs: map[string]any{
						"name": map[string]any{"old": "foo", "new": "bob"},
					},
					TxId:      &tx2,
					ChangedAt: now.Add(time.Minute),
				},
			},
			contains: []string{
				"Tx 1001 @ 2024-01-01T12:00:00Z",
				"Tx 1002 @ 2024-01-01T12:01:00Z",
				"INSERT",
				"UPDATE",
			},
		},
		{
			name: "no transaction ID",
			logs: []Log{
				{
					TableName: "settings",
					Operation: "UPDATE",
					RowData:   map[string]any{"key": "theme", "value": "dark"},
					ChangedAt: now,
				},
			},
			contains: []string{
				"Tx <none> @ 2024-01-01T12:00:00Z",
				"UPDATE",
				"settings",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderAscii(tt.logs)
			if tt.name == "empty logs" {
				require.Empty(t, got)
				return
			}
			for _, c := range tt.contains {
				require.Contains(t, got, c)
			}
		})
	}
}

var dbImage = "postgres:18"
var dbName = "postgres"
var dbUser = "test_user"
var dbPassword = "test_password"

func TestRecorder(t *testing.T) {

	testCases := []struct {
		name     string
		config   Config
		testFunc func(t *testing.T, db DB, recorder *Recorder)
	}{
		{
			name: "test setup returns no errors",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				err := recorder.Setup(t.Context(), db)

				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "test record insert on table",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				_, err := db.Exec(t.Context(), createSimpleTableSQL)

				require.Nil(t, err)

				err = recorder.Setup(t.Context(), db)

				require.Nil(t, err)

				_, err = db.Exec(t.Context(), "INSERT INTO simple_table (id, value) VALUES ($1, $2)", "1", "test")

				require.Nil(t, err)

				logs, err := recorder.GetLogs(t.Context(), db, []string{"simple_table"})

				require.Nil(t, err)

				require.Len(t, logs, 1)

				log := logs[0]

				require.Equal(t, "simple_table", log.TableName)
				require.Equal(t, "INSERT", log.Operation)
				require.Equal(t, map[string]any{"id": "1"}, log.Pk)
				require.Equal(t, map[string]any{"id": "1", "value": "test"}, log.RowData)
				require.Nil(t, log.Diffs)
			},
		},
		{
			name: "test complex types and multiple operations",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				_, err := db.Exec(t.Context(), createComplexTableSQL)
				require.Nil(t, err)

				err = recorder.Setup(t.Context(), db)
				require.Nil(t, err)

				now := time.Now().Truncate(time.Microsecond).UTC()
				_, err = db.Exec(t.Context(), `
					INSERT INTO complex_table (id, int_val, bool_val, json_val, time_val, numeric_val)
					VALUES ($1, $2, $3, $4, $5, $6)`,
					"uuid-1", 42, true, map[string]any{"foo": "bar"}, now, "123.45")
				require.Nil(t, err)

				_, err = db.Exec(t.Context(), `
					UPDATE complex_table SET int_val = 100, bool_val = FALSE, json_val = '{"foo": "baz"}'::JSONB
					WHERE id = 'uuid-1'`)
				require.Nil(t, err)

				_, err = db.Exec(t.Context(), "DELETE FROM complex_table WHERE id = 'uuid-1'")
				require.Nil(t, err)

				logs, err := recorder.GetLogs(t.Context(), db, []string{"complex_table"})
				require.Nil(t, err)
				require.Len(t, logs, 3)

				logInsert := logs[0]
				require.Equal(t, "INSERT", logInsert.Operation)
				require.Equal(t, "uuid-1", logInsert.Pk["id"])
				require.Equal(t, float64(42), logInsert.RowData["int_val"])
				require.Equal(t, true, logInsert.RowData["bool_val"])
				require.Equal(t, map[string]any{"foo": "bar"}, logInsert.RowData["json_val"])

				logUpdate := logs[1]
				require.Equal(t, "UPDATE", logUpdate.Operation)
				require.NotNil(t, logUpdate.Diffs)
				require.Equal(t, map[string]any{"old": float64(42), "new": float64(100)}, logUpdate.Diffs["int_val"])
				require.Equal(t, map[string]any{"old": true, "new": false}, logUpdate.Diffs["bool_val"])

				logDelete := logs[2]
				require.Equal(t, "DELETE", logDelete.Operation)
				require.Equal(t, "uuid-1", logDelete.Pk["id"])
			},
		},
		{
			name: "test composite primary key",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				_, err := db.Exec(t.Context(), `CREATE TABLE composite_pk (
					tenant_id TEXT,
					user_id TEXT,
					data TEXT,
					PRIMARY KEY (tenant_id, user_id)
				)`)
				require.Nil(t, err)

				err = recorder.Setup(t.Context(), db)
				require.Nil(t, err)

				_, err = db.Exec(t.Context(), "INSERT INTO composite_pk (tenant_id, user_id, data) VALUES ($1, $2, $3)", "t1", "u1", "some data")
				require.Nil(t, err)

				logs, err := recorder.GetLogs(t.Context(), db, []string{"composite_pk"})
				require.Nil(t, err)
				require.Len(t, logs, 1)

				require.Equal(t, map[string]any{"tenant_id": "t1", "user_id": "u1"}, logs[0].Pk)
			},
		},
		{
			name: "test table with no primary key",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				_, err := db.Exec(t.Context(), `CREATE TABLE no_pk (
					val TEXT
				)`)
				require.Nil(t, err)

				err = recorder.Setup(t.Context(), db)
				require.Nil(t, err)

				_, err = db.Exec(t.Context(), "INSERT INTO no_pk (val) VALUES ($1)", "foo")
				require.Nil(t, err)

				logs, err := recorder.GetLogs(t.Context(), db, []string{"no_pk"})
				require.Nil(t, err)
				require.Len(t, logs, 1)

				require.Equal(t, map[string]any{}, logs[0].Pk)
			},
		},
		{
			name: "test table with numeric primary key",
			testFunc: func(t *testing.T, db DB, recorder *Recorder) {
				_, err := db.Exec(t.Context(), `CREATE TABLE numeric_pk (
					id BIGINT PRIMARY KEY,
					val TEXT
				)`)
				require.Nil(t, err)

				err = recorder.Setup(t.Context(), db)
				require.Nil(t, err)

				_, err = db.Exec(t.Context(), "INSERT INTO numeric_pk (id, val) VALUES ($1, $2)", 123, "test")
				require.Nil(t, err)

				logs, err := recorder.GetLogs(t.Context(), db, []string{"numeric_pk"})
				require.Nil(t, err)
				require.Len(t, logs, 1)

				require.Equal(t, float64(123), logs[0].Pk["id"])
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dbContainer := setupPostgresContainer(t)

			defer testcontainers.TerminateContainer(dbContainer)

			testDBConnStr, err := dbContainer.ConnectionString(t.Context(), "sslmode=disable")

			if err != nil {
				t.Fatal(err)
			}

			testDBConn, err := pgx.Connect(t.Context(), testDBConnStr)

			if err != nil {
				t.Fatal(err)
			}

			defer testDBConn.Close(t.Context())

			testCase.testFunc(t, testDBConn, New(testCase.config))
		})
	}

}

func setupPostgresContainer(t *testing.T) *postgres.PostgresContainer {
	t.Helper()

	ctx := context.Background()

	postgresContainer, err := postgres.Run(
		ctx,
		dbImage,
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		postgres.BasicWaitStrategies(),
	)

	if err != nil {
		t.Fatal(err)
	}

	return postgresContainer
}

var createSimpleTableSQL = `CREATE TABLE simple_table
(
    id    TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

var createComplexTableSQL = `CREATE TABLE complex_table
(
    id          TEXT PRIMARY KEY,
    int_val     INTEGER,
    bool_val    BOOLEAN,
    json_val    JSONB,
    time_val    TIMESTAMPTZ,
    numeric_val NUMERIC
);
`
