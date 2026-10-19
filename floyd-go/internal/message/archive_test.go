package message

import (
	"context"
	"testing"

	"github.com/legacy-ai/floyd/internal/db"
	"github.com/stretchr/testify/require"
)

func TestSearchTechnicalArchive_SemanticFirewall(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Connect(ctx, t.TempDir())
	require.NoError(t, err)
	defer conn.Close()

	q := db.New(conn)
	svc := NewService(q, WithDB(conn))

	_, err = conn.ExecContext(ctx, "INSERT INTO sessions (id, title, created_at, updated_at) VALUES (?, ?, strftime('%s', 'now'), strftime('%s', 'now'))", "test-session-1", "Test Session")
	require.NoError(t, err)

	// Insert tool result with "bash"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'))",
		"msg-1", "test-session-1", "tool",
		`[{"type":"tool_result","data":{"tool_call_id":"tc-1","name":"bash","content":"output"}}]`,
	)
	require.NoError(t, err)

	results, err := svc.SearchTechnicalArchive(ctx, "/tmp", "bash", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "Should find tool result with bash")
}

func TestSearchTechnicalArchive_NoPlainText(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Connect(ctx, t.TempDir())
	require.NoError(t, err)
	defer conn.Close()

	q := db.New(conn)
	svc := NewService(q, WithDB(conn))

	_, err = conn.ExecContext(ctx, "INSERT INTO sessions (id, title, created_at, updated_at) VALUES (?, ?, strftime('%s', 'now'), strftime('%s', 'now'))", "test-session-2", "Test Session 2")
	require.NoError(t, err)

	_, err = conn.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'))",
		"msg-plain", "test-session-2", "user", `[{"type":"text","data":{"text":"I have a bug"}}]`,
	)
	require.NoError(t, err)

	results, err := svc.SearchTechnicalArchive(ctx, "/tmp", "bug", 10)
	require.NoError(t, err)
	require.Empty(t, results, "Plain text without code should be excluded")
}

func TestSearchTechnicalArchive_CodeBlockIncluded(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Connect(ctx, t.TempDir())
	require.NoError(t, err)
	defer conn.Close()

	q := db.New(conn)
	svc := NewService(q, WithDB(conn))

	_, err = conn.ExecContext(ctx, "INSERT INTO sessions (id, title, created_at, updated_at) VALUES (?, ?, strftime('%s', 'now'), strftime('%s', 'now'))", "test-session-3", "Test Session 3")
	require.NoError(t, err)

	// Using interpreted string with escaped backticks and quotes
	codeParts := "[{\"type\":\"text\",\"data\":{\"text\":\"Code:\\n```go\\nfmt.Println(\\\"hello\\\")\\n```\"}}]"
	_, err = conn.ExecContext(ctx,
		"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'))",
		"msg-code", "test-session-3", "user", codeParts,
	)
	require.NoError(t, err)

	results, err := svc.SearchTechnicalArchive(ctx, "/tmp", "Println", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "User message with code block should be included")
	// Fix: compare MessageRole to MessageRole, not string
	require.Equal(t, MessageRole("user"), results[0].Message.Role)
}

func TestSearchTechnicalArchive_Limit(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Connect(ctx, t.TempDir())
	require.NoError(t, err)
	defer conn.Close()

	q := db.New(conn)
	svc := NewService(q, WithDB(conn))

	_, err = conn.ExecContext(ctx, "INSERT INTO sessions (id, title, created_at, updated_at) VALUES (?, ?, strftime('%s', 'now'), strftime('%s', 'now'))", "test-session-4", "Test Session 4")
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		_, err = conn.ExecContext(ctx,
			"INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now'))",
			"msg-limit-"+string(rune('0'+i)), "test-session-4", "tool",
			`[{"type":"tool_result","data":{"tool_call_id":"tc-1","name":"test","content":"limit test data"}}]`,
		)
		require.NoError(t, err)
	}

	results, err := svc.SearchTechnicalArchive(ctx, "/tmp", "limit", 3)
	require.NoError(t, err)
	require.Len(t, results, 3, "Should return exactly 3 results")
}
