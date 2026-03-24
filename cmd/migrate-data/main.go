// Command migrate-data imports existing data from the Saiduler and Chatovadlo
// databases into the unified Botka database. It preserves UUIDs and IDs,
// remaps folder_id to project_id, and copies uploaded files.
//
// Usage:
//
//	go run cmd/migrate-data/main.go [--clean]
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
)

const (
	chatovadloUploads = "/home/pi/projects/chatovadlo/data/uploads"
	botkaUploads      = "/home/pi/projects/botka/data/uploads"
)

func main() {
	clean := flag.Bool("clean", false, "truncate target tables before import")
	saidulerDSN := flag.String("saiduler-dsn",
		"postgres://saiduler:saiduler@localhost:5432/saiduler?sslmode=disable",
		"Saiduler database DSN")
	chatovadloDSN := flag.String("chatovadlo-dsn",
		"postgres://botka_chat:BotkaChat2024Secure@localhost:5432/botka_chat?sslmode=disable",
		"Chatovadlo database DSN")
	botkaDSN := flag.String("botka-dsn",
		"postgres://botka:botka@localhost:5432/botka?sslmode=disable",
		"Botka database DSN")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	saidulerDB, err := openDB(*saidulerDSN, "saiduler")
	if err != nil {
		fatal("connect saiduler", err)
	}
	defer saidulerDB.Close()

	chatDB, err := openDB(*chatovadloDSN, "chatovadlo")
	if err != nil {
		fatal("connect chatovadlo", err)
	}
	defer chatDB.Close()

	botkaDB, err := openDB(*botkaDSN, "botka")
	if err != nil {
		fatal("connect botka", err)
	}
	defer botkaDB.Close()

	if *clean {
		if err := cleanTarget(botkaDB); err != nil {
			fatal("clean", err)
		}
	}

	if err := importSaiduler(saidulerDB, botkaDB); err != nil {
		fatal("import saiduler", err)
	}

	if err := importChatovadlo(chatDB, botkaDB); err != nil {
		fatal("import chatovadlo", err)
	}

	copyUploads()
	resetSequences(botkaDB)

	slog.Info("migration complete")
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func openDB(dsn, name string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", name, err)
	}
	slog.Info("connected", "db", name)
	return db, nil
}

func cleanTarget(db *sql.DB) error {
	slog.Info("cleaning target tables")
	_, err := db.Exec(`
		TRUNCATE projects, tasks, task_executions, personas, threads,
			messages, attachments, branch_selections, tags, thread_tags, memories CASCADE;
		UPDATE runner_state SET state = 'stopped', completed_count = 0, task_limit = NULL WHERE id = 1;
	`)
	if err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	slog.Info("target tables cleaned")
	return nil
}

// ---------------------------------------------------------------------------
// Saiduler import
// ---------------------------------------------------------------------------

func importSaiduler(src, dst *sql.DB) error {
	slog.Info("=== importing from saiduler ===")

	tx, err := dst.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	n, err := migrateProjects(src, tx)
	if err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	slog.Info("imported saiduler projects", "count", n)

	n, err = migrateTasks(src, tx)
	if err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	slog.Info("imported tasks", "count", n)

	n, err = migrateTaskExecutions(src, tx)
	if err != nil {
		return fmt.Errorf("task_executions: %w", err)
	}
	slog.Info("imported task_executions", "count", n)

	if err := migrateRunnerState(src, tx); err != nil {
		return fmt.Errorf("runner_state: %w", err)
	}

	return tx.Commit()
}

func migrateProjects(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`
		SELECT id, name, path, branch_strategy, verification_command, active, created_at, updated_at
		FROM projects ORDER BY created_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, name, path, strategy string
			verCmd                   *string
			active                   bool
			createdAt, updatedAt     time.Time
		)
		if err := rows.Scan(&id, &name, &path, &strategy, &verCmd, &active, &createdAt, &updatedAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		// Remove any auto-discovered project with same path but different UUID.
		if _, err := tx.Exec(`DELETE FROM projects WHERE path = $1 AND id != $2`, path, id); err != nil {
			slog.Warn("delete conflicting project", "path", path, "error", err)
		}
		_, err := tx.Exec(`
			INSERT INTO projects (id, name, path, branch_strategy, verification_command, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (id) DO NOTHING
		`, id, name, path, strategy, verCmd, active, createdAt, updatedAt)
		if err != nil {
			slog.Warn("skip project", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateTasks(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`
		SELECT id, title, spec, status, priority, project_id, failure_reason, retry_count, created_at, updated_at
		FROM tasks ORDER BY created_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, title, spec, projectID string
			status                     string
			priority, retryCount       int
			failureReason              *string
			createdAt, updatedAt       time.Time
		)
		if err := rows.Scan(&id, &title, &spec, &status, &priority, &projectID, &failureReason, &retryCount, &createdAt, &updatedAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO tasks (id, title, spec, status, priority, project_id, failure_reason, retry_count, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (id) DO NOTHING
		`, id, title, spec, status, priority, projectID, failureReason, retryCount, createdAt, updatedAt)
		if err != nil {
			slog.Warn("skip task", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateTaskExecutions(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`
		SELECT id, task_id, attempt, started_at, finished_at, exit_code, cost_usd, duration_ms, summary, error_message, created_at
		FROM task_executions ORDER BY created_at
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, taskID            string
			attempt               int
			startedAt             time.Time
			finishedAt            *time.Time
			exitCode              *int
			costUSD               *float64
			durationMs            *int64
			summary, errorMessage *string
			createdAt             time.Time
		)
		if err := rows.Scan(&id, &taskID, &attempt, &startedAt, &finishedAt, &exitCode, &costUSD, &durationMs, &summary, &errorMessage, &createdAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO task_executions (id, task_id, attempt, started_at, finished_at, exit_code, cost_usd, duration_ms, summary, error_message, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO NOTHING
		`, id, taskID, attempt, startedAt, finishedAt, exitCode, costUSD, durationMs, summary, errorMessage, createdAt)
		if err != nil {
			slog.Warn("skip task_execution", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateRunnerState(src *sql.DB, tx *sql.Tx) error {
	var (
		state          string
		completedCount int
		taskLimit      int
	)
	// runner_state table may not exist in older Saiduler instances.
	var exists bool
	if err := src.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.tables WHERE table_name = 'runner_state'
	)`).Scan(&exists); err != nil || !exists {
		slog.Info("no runner_state table in saiduler, skipping")
		return nil
	}

	err := src.QueryRow(`SELECT state, completed_count, task_limit FROM runner_state WHERE id = 1`).
		Scan(&state, &completedCount, &taskLimit)
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Info("no runner_state row in saiduler, skipping")
			return nil
		}
		return err
	}

	// Saiduler stores task_limit as NOT NULL DEFAULT 0; Botka uses nullable (NULL = no limit).
	var botkaLimit *int
	if taskLimit > 0 {
		botkaLimit = &taskLimit
	}

	_, err = tx.Exec(`
		UPDATE runner_state SET state = $1, completed_count = $2, task_limit = $3, updated_at = NOW()
		WHERE id = 1
	`, state, completedCount, botkaLimit)
	if err != nil {
		return err
	}
	slog.Info("imported runner_state", "state", state, "completed", completedCount)
	return nil
}

// ---------------------------------------------------------------------------
// Chatovadlo import
// ---------------------------------------------------------------------------

func importChatovadlo(src, dst *sql.DB) error {
	slog.Info("=== importing from chatovadlo ===")

	tx, err := dst.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Folders → project mapping
	folderMap, err := migrateFolders(src, tx)
	if err != nil {
		return fmt.Errorf("folders: %w", err)
	}
	slog.Info("mapped folders to projects", "count", len(folderMap))

	// 2. Personas
	n, err := migratePersonas(src, tx)
	if err != nil {
		return fmt.Errorf("personas: %w", err)
	}
	slog.Info("imported personas", "count", n)

	// 3. Tags
	n, err = migrateTags(src, tx)
	if err != nil {
		return fmt.Errorf("tags: %w", err)
	}
	slog.Info("imported tags", "count", n)

	// 4. Threads (remap folder_id → project_id)
	n, err = migrateThreads(src, tx, folderMap)
	if err != nil {
		return fmt.Errorf("threads: %w", err)
	}
	slog.Info("imported threads", "count", n)

	// 5. Messages (ordered by id to respect parent_id references)
	n, err = migrateMessages(src, tx)
	if err != nil {
		return fmt.Errorf("messages: %w", err)
	}
	slog.Info("imported messages", "count", n)

	// 6. Attachments
	n, err = migrateAttachments(src, tx)
	if err != nil {
		return fmt.Errorf("attachments: %w", err)
	}
	slog.Info("imported attachments", "count", n)

	// 7. Branch selections
	n, err = migrateBranchSelections(src, tx)
	if err != nil {
		return fmt.Errorf("branch_selections: %w", err)
	}
	slog.Info("imported branch_selections", "count", n)

	// 8. Thread tags
	n, err = migrateThreadTags(src, tx)
	if err != nil {
		return fmt.Errorf("thread_tags: %w", err)
	}
	slog.Info("imported thread_tags", "count", n)

	// 9. Memories
	n, err = migrateMemories(src, tx)
	if err != nil {
		return fmt.Errorf("memories: %w", err)
	}
	slog.Info("imported memories", "count", n)

	return tx.Commit()
}

// migrateFolders reads Chatovadlo folders and maps them to Botka projects.
// Returns folder_id → project_id (UUID string).
func migrateFolders(src *sql.DB, tx *sql.Tx) (map[int64]string, error) {
	rows, err := src.Query(`
		SELECT id, name, sort_order, directory_path, claude_md, created_at, updated_at
		FROM folders ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folderMap := make(map[int64]string)
	for rows.Next() {
		var (
			folderID             int64
			name                 string
			sortOrder            int
			directoryPath        *string
			claudeMD             string
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&folderID, &name, &sortOrder, &directoryPath, &claudeMD, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}

		// Try to match by directory_path to an existing project in target.
		if directoryPath != nil && *directoryPath != "" {
			var existingID string
			err := tx.QueryRow(`SELECT id FROM projects WHERE path = $1`, *directoryPath).Scan(&existingID)
			if err == nil {
				// Match found: update claude_md and sort_order from folder.
				if _, err := tx.Exec(`UPDATE projects SET claude_md = $1, sort_order = $2 WHERE id = $3`,
					claudeMD, sortOrder, existingID); err != nil {
					slog.Warn("update project from folder", "folder_id", folderID, "error", err)
				}
				folderMap[folderID] = existingID
				slog.Info("matched folder to project", "folder", name, "project_id", existingID)
				continue
			}
		}

		// No match: create a new project from folder data.
		path := fmt.Sprintf("/chatovadlo/folders/%d", folderID)
		if directoryPath != nil && *directoryPath != "" {
			path = *directoryPath
		}

		var newID string
		err := tx.QueryRow(`
			INSERT INTO projects (name, path, active, claude_md, sort_order, created_at, updated_at)
			VALUES ($1, $2, false, $3, $4, $5, $6)
			ON CONFLICT (path) DO UPDATE SET claude_md = EXCLUDED.claude_md, sort_order = EXCLUDED.sort_order
			RETURNING id
		`, name, path, claudeMD, sortOrder, createdAt, updatedAt).Scan(&newID)
		if err != nil {
			slog.Warn("create project from folder", "folder_id", folderID, "name", name, "error", err)
			continue
		}
		folderMap[folderID] = newID
		slog.Info("created project from folder", "folder", name, "project_id", newID)
	}
	return folderMap, rows.Err()
}

func migratePersonas(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`
		SELECT id, name, system_prompt, default_model, icon, starter_message, sort_order, created_at, updated_at
		FROM personas ORDER BY id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id                   int64
			name, sysPrompt      string
			defModel, icon       *string
			starter              *string
			sortOrder            int
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&id, &name, &sysPrompt, &defModel, &icon, &starter, &sortOrder, &createdAt, &updatedAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO personas (id, name, system_prompt, default_model, icon, starter_message, sort_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO NOTHING
		`, id, name, sysPrompt, defModel, icon, starter, sortOrder, createdAt, updatedAt)
		if err != nil {
			slog.Warn("skip persona", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateTags(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`SELECT id, name, color, created_at FROM tags ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id        int64
			name      string
			color     string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &color, &createdAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO tags (id, name, color, created_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO NOTHING
		`, id, name, color, createdAt)
		if err != nil {
			slog.Warn("skip tag", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateThreads(src *sql.DB, tx *sql.Tx, folderMap map[int64]string) (int, error) {
	rows, err := src.Query(`
		SELECT id, title, model, system_prompt, persona_id, persona_name, folder_id,
			pinned, archived, claude_session_id, created_at, updated_at
		FROM threads ORDER BY id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id                   int64
			title                string
			model                *string
			sysPrompt            string
			personaID            *int64
			personaName          string
			folderID             *int64
			pinned, archived     bool
			claudeSessionID      *string
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&id, &title, &model, &sysPrompt, &personaID, &personaName, &folderID,
			&pinned, &archived, &claudeSessionID, &createdAt, &updatedAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}

		// Remap folder_id → project_id.
		var projectID *string
		if folderID != nil {
			if pid, ok := folderMap[*folderID]; ok {
				projectID = &pid
			} else {
				slog.Warn("unmapped folder_id", "thread_id", id, "folder_id", *folderID)
			}
		}

		_, err := tx.Exec(`
			INSERT INTO threads (id, title, model, system_prompt, persona_id, persona_name, project_id,
				pinned, archived, claude_session_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (id) DO NOTHING
		`, id, title, model, sysPrompt, personaID, personaName, projectID,
			pinned, archived, claudeSessionID, createdAt, updatedAt)
		if err != nil {
			slog.Warn("skip thread", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateMessages(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`
		SELECT id, thread_id, role, content, parent_id, thinking, thinking_duration_ms,
			prompt_tokens, completion_tokens, created_at
		FROM messages ORDER BY id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, threadID                   int64
			role, content                  string
			parentID                       *int64
			thinking                       *string
			thinkingDuration               *int
			promptTokens, completionTokens *int
			createdAt                      time.Time
		)
		if err := rows.Scan(&id, &threadID, &role, &content, &parentID, &thinking,
			&thinkingDuration, &promptTokens, &completionTokens, &createdAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO messages (id, thread_id, role, content, parent_id, thinking,
				thinking_duration_ms, prompt_tokens, completion_tokens, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (id) DO NOTHING
		`, id, threadID, role, content, parentID, thinking,
			thinkingDuration, promptTokens, completionTokens, createdAt)
		if err != nil {
			slog.Warn("skip message", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateAttachments(src *sql.DB, tx *sql.Tx) (int, error) {
	// Chatovadlo column "filename" maps to Botka "stored_name".
	rows, err := src.Query(`
		SELECT id, message_id, filename, original_name, mime_type, size, created_at
		FROM attachments ORDER BY id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, messageID int64
			storedName    string
			originalName  string
			mimeType      string
			size          int64
			createdAt     time.Time
		)
		if err := rows.Scan(&id, &messageID, &storedName, &originalName, &mimeType, &size, &createdAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO attachments (id, message_id, stored_name, original_name, mime_type, size, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO NOTHING
		`, id, messageID, storedName, originalName, mimeType, size, createdAt)
		if err != nil {
			slog.Warn("skip attachment", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateBranchSelections(src *sql.DB, tx *sql.Tx) (int, error) {
	// Chatovadlo has composite PK (thread_id, fork_message_id).
	// Botka has BIGSERIAL id + UNIQUE(thread_id, fork_message_id).
	rows, err := src.Query(`
		SELECT thread_id, fork_message_id, selected_child_id
		FROM branch_selections ORDER BY thread_id, fork_message_id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var threadID, forkMsgID, selectedChildID int64
		if err := rows.Scan(&threadID, &forkMsgID, &selectedChildID); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO branch_selections (thread_id, fork_message_id, selected_child_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (thread_id, fork_message_id) DO NOTHING
		`, threadID, forkMsgID, selectedChildID)
		if err != nil {
			slog.Warn("skip branch_selection", "thread_id", threadID, "fork", forkMsgID, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateThreadTags(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`SELECT thread_id, tag_id FROM thread_tags`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var threadID, tagID int64
		if err := rows.Scan(&threadID, &tagID); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO thread_tags (thread_id, tag_id) VALUES ($1, $2)
			ON CONFLICT (thread_id, tag_id) DO NOTHING
		`, threadID, tagID)
		if err != nil {
			slog.Warn("skip thread_tag", "thread_id", threadID, "tag_id", tagID, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

func migrateMemories(src *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := src.Query(`SELECT id, content, created_at, updated_at FROM memories ORDER BY created_at`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id                   string
			content              string
			createdAt, updatedAt time.Time
		)
		if err := rows.Scan(&id, &content, &createdAt, &updatedAt); err != nil {
			return count, fmt.Errorf("scan: %w", err)
		}
		_, err := tx.Exec(`
			INSERT INTO memories (id, content, created_at, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO NOTHING
		`, id, content, createdAt, updatedAt)
		if err != nil {
			slog.Warn("skip memory", "id", id, "error", err)
			continue
		}
		count++
	}
	return count, rows.Err()
}

// ---------------------------------------------------------------------------
// Upload file copy
// ---------------------------------------------------------------------------

func copyUploads() {
	entries, err := os.ReadDir(chatovadloUploads)
	if err != nil {
		slog.Info("no uploads to copy", "src", chatovadloUploads, "error", err)
		return
	}

	if err := os.MkdirAll(botkaUploads, 0o755); err != nil {
		slog.Warn("create upload dir", "error", err)
		return
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(chatovadloUploads, entry.Name())
		dstPath := filepath.Join(botkaUploads, entry.Name())

		if _, err := os.Stat(dstPath); err == nil {
			continue // already exists
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			slog.Warn("copy file", "file", entry.Name(), "error", err)
			continue
		}
		count++
	}
	slog.Info("copied uploads", "count", count)
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	return err
}

// ---------------------------------------------------------------------------
// Sequence reset
// ---------------------------------------------------------------------------

// resetSequences advances BIGSERIAL sequences past the maximum imported ID
// so that future inserts don't collide with migrated rows.
func resetSequences(db *sql.DB) {
	seqs := []struct {
		seq   string
		table string
	}{
		{"personas_id_seq", "personas"},
		{"threads_id_seq", "threads"},
		{"messages_id_seq", "messages"},
		{"attachments_id_seq", "attachments"},
		{"branch_selections_id_seq", "branch_selections"},
		{"tags_id_seq", "tags"},
	}
	for _, s := range seqs {
		_, err := db.Exec(fmt.Sprintf(
			`SELECT setval('%s', COALESCE((SELECT MAX(id) FROM %s), 0) + 1, false)`,
			s.seq, s.table,
		))
		if err != nil {
			slog.Warn("reset sequence", "seq", s.seq, "error", err)
		}
	}
	slog.Info("sequences reset")
}
